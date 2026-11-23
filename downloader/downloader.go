package downloader

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"futcor_predict/models"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"gopkg.in/yaml.v3"
)

type TableConfig struct {
	Name               string `yaml:"name"`
	TableID            string `yaml:"table_id"`
	Referer            string `yaml:"referer"`
	Category           string `yaml:"category"`
	League             string `yaml:"league"`
	FilterCountry      string `yaml:"filter_country"` // Opcional: filtra col[2] (Country) para desambiguar ligas homónimas (ej. Serie A Italia vs Brasil)
	FilterLeague       string `yaml:"filter_league"`
	FilterLeagueColumn *int   `yaml:"filter_league_column"` // Índice de columna donde vive la liga. nil = default 2
	KeepColumns        []int  `yaml:"keep_columns"`
}

type ResponseData struct {
	Draw            int             `json:"draw"`
	RecordsTotal    string          `json:"recordsTotal"`
	RecordsFiltered string          `json:"recordsFiltered"`
	Data            [][]interface{} `json:"data"`
}

type pageNonces struct {
	byName       map[string]string
	defaultNonce string
}

// httpErr permite distinguir el código HTTP en errores para manejo de WAF y reportes.
type httpErr struct {
	Status int
	URL    string
}

func (e *httpErr) Error() string {
	return fmt.Sprintf("HTTP %d en %s", e.Status, e.URL)
}

const defaultUserAgent = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36"

var commonHeaders = map[string]string{
	"accept":           "application/json, text/javascript, */*; q=0.01",
	"accept-encoding":  "gzip, deflate, zstd",
	"accept-language":  "es-419,es;q=0.9",
	"origin":           "https://footiqo.com",
	"user-agent":       defaultUserAgent,
	"x-requested-with": "XMLHttpRequest",
}

var (
	reNonceNameFirst  = regexp.MustCompile(`(?i)name="(wdtNonce[^"]*)"[^>]*value="([^"]+)"`)
	reNonceValueFirst = regexp.MustCompile(`(?i)value="([^"]+)"[^>]*name="(wdtNonce[^"]*)"`)
)

// getUserAgent devuelve FOOTIQO_UA si está definido para mantener coherencia con cookies externas (cf_clearance).
func getUserAgent() string {
	if ua := strings.TrimSpace(os.Getenv("FOOTIQO_UA")); ua != "" {
		return ua
	}
	return commonHeaders["user-agent"]
}

// applyBrowserHeaders fija headers coherentes con un Chrome 120 en Android,
// incluyendo sec-ch-ua y sec-fetch-* que los WAF modernos usan como señal.
func applyBrowserHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", getUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "es-419,es;q=0.9,en;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, deflate, zstd")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("sec-ch-ua", `"Chromium";v="120", "Google Chrome";v="120", "Not_A Brand";v="99"`)
	req.Header.Set("sec-ch-ua-mobile", "?1")
	req.Header.Set("sec-ch-ua-platform", `"Android"`)
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("sec-fetch-user", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
}

// debugLogRequest imprime headers y payload enviados cuando DEBUG_PAYLOAD=1 está activo.
func debugLogRequest(req *http.Request, payload string) {
	if os.Getenv("DEBUG_PAYLOAD") != "1" {
		return
	}
	fmt.Printf("  🛠️ [DEBUG_PAYLOAD] POST %s\n", req.URL.String())
	fmt.Printf("     Headers: %v\n", req.Header)
	fmt.Printf("     Payload: %s\n", payload)
}

// SaveCookies persiste las cookies del jar asociadas al dominio footiqo.com en disco.
func SaveCookies(jar http.CookieJar, path string) error {
	if jar == nil || strings.TrimSpace(path) == "" {
		return nil
	}
	u, err := url.Parse("https://footiqo.com/")
	if err != nil {
		return err
	}
	cookies := jar.Cookies(u)
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializando cookies: %w", err)
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("error creando directorio de cookies: %w", err)
		}
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("error guardando cookies en %s: %w", path, err)
	}
	return nil
}

// LoadCookies carga cookies guardadas desde disco al CookieJar para footiqo.com.
func LoadCookies(jar http.CookieJar, path string) error {
	if jar == nil || strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error leyendo cookies de %s: %w", path, err)
	}
	var cookies []*http.Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return fmt.Errorf("error parseando cookies de %s: %w", path, err)
	}
	u, err := url.Parse("https://footiqo.com/")
	if err != nil {
		return err
	}
	jar.SetCookies(u, cookies)
	return nil
}

// jitterSleep duerme entre min y max para romper patrones rítmicos detectables.
func jitterSleep(min, max time.Duration) {
	if max <= min {
		time.Sleep(min)
		return
	}
	delta := max - min
	time.Sleep(min + time.Duration(rand.Int63n(int64(delta))))
}

// readAndDrainBody lee hasta maxBytes y drena el remanente para permitir reutilizar la conexión TCP/TLS.
func readAndDrainBody(body io.ReadCloser, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBytes))
	_, _ = io.Copy(io.Discard, body)
	return data, err
}

// warmUp visita la home y una sección para que el cookiejar reciba cookies de sesión
// antes de golpear URLs profundas. Sin esto, la primera petición "parece" de bot.
func warmUp(client *http.Client) error {
	urls := []string{
		"https://footiqo.com/es/",
		"https://footiqo.com/es/base-de-datos/",
	}
	for _, u := range urls {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return err
		}
		applyBrowserHeaders(req, "https://footiqo.com/")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body) // drenar para reutilizar conexión
		resp.Body.Close()
		jitterSleep(400*time.Millisecond, 800*time.Millisecond)
	}
	return nil
}

func inspectHTTPResponse(stage, targetURL string, resp *http.Response, rawBody []byte) {
	body, err := decompressBody(resp, rawBody)
	if err != nil {
		body = rawBody
	}
	preview := strings.ReplaceAll(string(body), "\n", " ")
	preview = strings.ReplaceAll(preview, "\r", " ")
	if len(preview) > 250 {
		preview = preview[:250]
	}
	fmt.Printf("  🔎 [%s] Status=%d URL=%s\n", stage, resp.StatusCode, targetURL)
	fmt.Printf("     Server=%q | Content-Type=%q | Content-Encoding=%q\n",
		resp.Header.Get("Server"),
		resp.Header.Get("Content-Type"),
		resp.Header.Get("Content-Encoding"),
	)
	if v := resp.Header.Get("CF-Mitigated"); v != "" {
		fmt.Printf("     CF-Mitigated=%q\n", v)
	}
	fmt.Printf("     Body[:250]=%q\n", preview)
}

func fetchPageNonces(client *http.Client, referer string) (*pageNonces, error) {
	req, err := http.NewRequest("GET", referer, nil)
	if err != nil {
		return nil, err
	}
	applyBrowserHeaders(req, "https://footiqo.com/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := readAndDrainBody(resp.Body, 512*1024)
		if resp.StatusCode == http.StatusForbidden || os.Getenv("DEBUG_HTTP") == "1" {
			inspectHTTPResponse("GET_NONCE", referer, resp, bodyBytes)
		}
		return nil, &httpErr{Status: resp.StatusCode, URL: referer}
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo HTML: %w", err)
	}

	body, err := decompressBody(resp, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("error descomprimiendo HTML: %w", err)
	}

	html := string(body)
	result := &pageNonces{
		byName: make(map[string]string),
	}

	for _, m := range reNonceNameFirst.FindAllStringSubmatch(html, -1) {
		if len(m) >= 3 {
			name, val := m[1], m[2]
			result.byName[name] = val
			if result.defaultNonce == "" {
				result.defaultNonce = val
			}
		}
	}

	for _, m := range reNonceValueFirst.FindAllStringSubmatch(html, -1) {
		if len(m) >= 3 {
			val, name := m[1], m[2]
			if _, exists := result.byName[name]; !exists {
				result.byName[name] = val
			}
			if result.defaultNonce == "" {
				result.defaultNonce = val
			}
		}
	}

	if result.defaultNonce == "" {
		return nil, fmt.Errorf("no se encontró el nonce en %s (posible bloqueo o cambio de web)", referer)
	}

	return result, nil
}

func (p *pageNonces) getForTable(tableID string) string {
	// Priorizar nonce específico de la tabla (ej. wdtNonceFrontendServerSide_365)
	suffix := "_" + tableID
	for name, val := range p.byName {
		if strings.HasSuffix(name, suffix) {
			return val
		}
	}
	for name, val := range p.byName {
		if strings.Contains(name, tableID) {
			return val
		}
	}
	return p.defaultNonce
}

func DescargarTodo(outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("no se pudo crear el directorio %s: %w", outDir, err)
	}

	yamlPath := filepath.Join(outDir, "tables.yaml")
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		return fmt.Errorf("falta el archivo de configuración: %s", yamlPath)
	}

	yamlFile, err := os.ReadFile(yamlPath)
	if err != nil {
		return fmt.Errorf("error leyendo tables.yaml: %v", err)
	}

	var tables []TableConfig
	if err := yaml.Unmarshal(yamlFile, &tables); err != nil {
		return fmt.Errorf("error parseando YAML: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	cookiesPath := strings.TrimSpace(os.Getenv("COOKIES_PATH"))
	if cookiesPath != "" {
		if _, err := os.Stat(cookiesPath); err == nil {
			if err := LoadCookies(jar, cookiesPath); err != nil {
				fmt.Printf("  ⚠️  no se pudieron cargar cookies desde %s: %v\n", cookiesPath, err)
			} else {
				fmt.Printf("  🍪 Cookies cargadas desde %s\n", cookiesPath)
			}
		}
		defer func() {
			if err := SaveCookies(jar, cookiesPath); err != nil {
				fmt.Printf("  ⚠️  no se pudieron guardar cookies en %s: %v\n", cookiesPath, err)
			}
		}()
	}

	client := &http.Client{Jar: jar, Timeout: 60 * time.Second}

	// WARM-UP: sembrar cookies de sesión antes de las peticiones profundas.
	if err := warmUp(client); err != nil {
		fmt.Printf("  ⚠️  warm-up falló: %v (continuando)\n", err)
	}

	nonceCache := make(map[string]*pageNonces)
	rawTableCache := make(map[string][][]interface{})
	failedReferers := make(map[string]error)

	exitos := 0
	errores := 0
	errores403 := 0
	ultimo403 := ""
	var ultimoError error

	for i, cfg := range tables {
		fmt.Printf("[%d/%d] 📥 %s\n", i+1, len(tables), cfg.Name)

		rows, fromCache, err := getTableRowsWithRetry(client, cfg, nonceCache, rawTableCache, failedReferers)
		if err != nil {
			fmt.Printf("  ❌ Error: %v\n", err)
			errores++
			ultimoError = err

			var he *httpErr
			if errors.As(err, &he) && he.Status == http.StatusForbidden {
				errores403++
				ultimo403 = he.URL
				if !fromCache {
					// Pausa moderada para no acelerar restricciones del WAF sin abortar el resto de tablas.
					fmt.Printf("  ⏳ HTTP 403 en %s; aplicando pausa de enfriamiento (6s ± 2s)...\n", he.URL)
					jitterSleep(4*time.Second, 8*time.Second)
				}
				continue
			}

			if !fromCache {
				jitterSleep(2500*time.Millisecond, 4000*time.Millisecond)
			}
			continue
		}

		if err := saveTableCSV(cfg, rows, outDir); err != nil {
			fmt.Printf("  ❌ Error guardando CSV: %v\n", err)
			errores++
			ultimoError = err
		} else {
			if fromCache {
				fmt.Printf("  ✅ Guardado (desde caché)\n")
			} else {
				fmt.Printf("  ✅ Guardado\n")
			}
			exitos++
		}

		if !fromCache {
			jitterSleep(2500*time.Millisecond, 4000*time.Millisecond)
		}
	}

	if exitos == 0 {
		return fmt.Errorf("fallaron todas las descargas. Último error: %v", ultimoError)
	}

	if syncErr := models.SyncEquiposCSV(outDir); syncErr != nil {
		fmt.Printf("  ⚠️  Error sincronizando equipos.csv: %v\n", syncErr)
	} else {
		fmt.Println("  ✅ equipos.csv actualizado automáticamente")
	}

	if errores > 0 {
		// Si hubo errores 403, los reportamos para saber que fue el WAF/Cloudflare
		if errores403 > 0 {
			return fmt.Errorf("descarga parcial: %d exitosas, %d fallidas. Hubo %d bloqueos 403 (WAF) en: %s", exitos, errores, errores403, ultimo403)
		}
		return fmt.Errorf("descarga parcial: %d exitosas, %d fallidas (Último error: %v)", exitos, errores, ultimoError)
	}

	return nil
}

func getTableRowsWithRetry(
	client *http.Client,
	cfg TableConfig,
	nonceCache map[string]*pageNonces,
	rawTableCache map[string][][]interface{},
	failedReferers map[string]error,
) ([][]interface{}, bool, error) {
	if cachedRows, ok := rawTableCache[cfg.TableID]; ok {
		return cachedRows, true, nil
	}
	if failedReferers != nil {
		if prevErr, blocked := failedReferers[cfg.Referer]; blocked {
			return nil, true, prevErr
		}
	}

	const maxAttempts = 2
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			// Invalidar nonce en reintento y esperar con backoff progresivo + jitter
			delete(nonceCache, cfg.Referer)
			jitterSleep(
				time.Duration(attempt)*3000*time.Millisecond,
				time.Duration(attempt)*4500*time.Millisecond,
			)
		}

		nonces, exists := nonceCache[cfg.Referer]
		if !exists {
			var err error
			nonces, err = fetchPageNonces(client, cfg.Referer)
			if err != nil {
				lastErr = fmt.Errorf("error obteniendo nonce: %w", err)
				// Si Cloudflare responde 403 en el GET de la página, no repetir de inmediato
				// ni volver a golpear el mismo Referer en esta corrida.
				var he *httpErr
				if errors.As(err, &he) && he.Status == http.StatusForbidden {
					if failedReferers != nil {
						failedReferers[cfg.Referer] = lastErr
					}
					break
				}
				continue
			}
			nonceCache[cfg.Referer] = nonces
			jitterSleep(1800*time.Millisecond, 2800*time.Millisecond)
		}

		nonce := nonces.getForTable(cfg.TableID)
		rows, err := fetchTableRows(client, cfg, nonce)
		if err != nil {
			lastErr = err
			var he *httpErr
			if errors.As(err, &he) && he.Status == http.StatusForbidden {
				// Invalidar nonce por si expiró, sin martillar en bucle.
				delete(nonceCache, cfg.Referer)
				break
			}
			continue
		}

		rawTableCache[cfg.TableID] = rows
		return rows, false, nil
	}

	return nil, false, lastErr
}

func fetchTableRows(client *http.Client, cfg TableConfig, nonce string) ([][]interface{}, error) {
	endpoint := fmt.Sprintf("https://footiqo.com/wp-admin/admin-ajax.php?action=get_wdtable&table_id=%s", cfg.TableID)
	payload := buildPayload(cfg.TableID, cfg.Category, nonce, 2000)

	req, err := http.NewRequest("POST", endpoint, bytes.NewBufferString(payload))
	if err != nil {
		return nil, err
	}

	for k, v := range commonHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", getUserAgent())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Referer", cfg.Referer)

	debugLogRequest(req, payload)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rawBody, _ := readAndDrainBody(resp.Body, 512*1024)
		if resp.StatusCode == http.StatusForbidden || os.Getenv("DEBUG_HTTP") == "1" {
			inspectHTTPResponse("POST_AJAX", endpoint, resp, rawBody)
		}
		return nil, &httpErr{Status: resp.StatusCode, URL: endpoint}
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	body, err := decompressBody(resp, rawBody)
	if err != nil {
		return nil, fmt.Errorf("error descomprimiendo: %w", err)
	}

	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("respuesta vacía (0 bytes) en %s [CT=%s]", cfg.Name, resp.Header.Get("Content-Type"))
	}

	var result ResponseData
	if err := json.Unmarshal(body, &result); err != nil {
		preview := string(body)
		if len(preview) > 160 {
			preview = preview[:160]
		}
		return nil, fmt.Errorf("JSON inválido en %s (%d bytes): %v | inicio: %q", cfg.Name, len(body), err, preview)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no hay datos en %s (posible nonce inválido o sin partidos)", cfg.Name)
	}

	return result.Data, nil
}

func saveTableCSV(cfg TableConfig, rawRows [][]interface{}, outDir string) error {
	rows := rawRows
	cat := strings.ToLower(strings.TrimSpace(cfg.Category))

	// FILTRO POR LIGA
	if cfg.FilterLeague != "" {
		// Columna donde vive la liga: 2 por defecto (compatibilidad con tablas xG/individuales),
		// configurable para tablas combinadas (índice 3 en BTTS/Córners/WDL, índice 2 en xG).
		col := 2
		if cfg.FilterLeagueColumn != nil {
			col = *cfg.FilterLeagueColumn
		}

		if os.Getenv("DEBUG_PAYLOAD") == "1" && len(rows) > 0 {
			fmt.Printf("  🔍 Estructura de tabla %s: ", cfg.TableID)
			for i, v := range rows[0] {
				fmt.Printf("[%d]=%v ", i, v)
			}
			fmt.Println()
		}

		targetCountry := strings.TrimSpace(cfg.FilterCountry)
		targetLeague := strings.TrimSpace(cfg.FilterLeague)
		var filtered [][]interface{}
		uniqueLeagues := make(map[string]bool)

		for _, row := range rows {
			if targetCountry != "" {
				if len(row) <= 2 {
					continue
				}
				country, ok := row[2].(string)
				if !ok {
					country = fmt.Sprintf("%v", row[2])
				}
				if !strings.EqualFold(strings.TrimSpace(country), targetCountry) {
					continue
				}
			}

			if col < 0 || len(row) <= col {
				continue
			}
			league, ok := row[col].(string)
			if !ok {
				league = fmt.Sprintf("%v", row[col])
			}
			cleanLeague := strings.TrimSpace(league)
			if cleanLeague != "" {
				uniqueLeagues[cleanLeague] = true
			}
			if cleanLeague == targetLeague {
				filtered = append(filtered, row)
			}
		}
		if len(filtered) == 0 {
			disponibles := make([]string, 0, len(uniqueLeagues))
			for l := range uniqueLeagues {
				disponibles = append(disponibles, l)
			}
			sort.Strings(disponibles)
			return fmt.Errorf("no se encontraron filas con liga '%s' en col[%d]. Ligas disponibles: %v",
				cfg.FilterLeague, col, disponibles)
		}
		if os.Getenv("DEBUG_PAYLOAD") == "1" {
			fmt.Printf("  🔎 Filtradas %d filas con liga '%s' en col[%d] (primera fila: %v)\n",
				len(filtered), targetLeague, col, filtered[0])
		}
		rows = filtered
	}

	// FILTRO COLUMNAS
	if len(cfg.KeepColumns) > 0 {
		var selected [][]interface{}
		for _, row := range rows {
			var newRow []interface{}
			for _, idx := range cfg.KeepColumns {
				if idx >= 0 && idx < len(row) {
					newRow = append(newRow, row[idx])
				} else {
					newRow = append(newRow, "")
				}
			}
			selected = append(selected, newRow)
		}
		rows = selected
	}

	// FILTRO XG
	if cat == "xg" {
		var filtered [][]interface{}
		for _, row := range rows {
			if len(row) > 0 {
				period, ok := row[0].(string)
				if ok && period == "Current season" {
					filtered = append(filtered, row)
				}
			}
		}
		if len(filtered) > 0 {
			rows = filtered
		}
	}

	// GUARDAR CSV
	outputPath := filepath.Join(outDir, cfg.Name+".csv")
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)

	var headers []string
	switch cat {
	case "btts":
		headers = []string{"Team", "Matches Played", "BTTS YES %", "BTTS NO %"}
	case "corners":
		headers = []string{"Team", "Matches Played", "Avg Corners Taken", "Avg Corners Conceded",
			"Over 7.5 (%)", "Under 7.5 (%)", "Over 8.5 (%)", "Under 8.5 (%)",
			"Over 9.5 (%)", "Under 9.5 (%)", "Over 10.5 (%)", "Under 10.5 (%)"}
	case "wdl":
		headers = []string{"Team", "Matches Played", "Win %", "Draw %", "Loss %"}
	case "xg":
		if len(cfg.KeepColumns) == 11 {
			headers = []string{"Period", "Country", "League", "Team", "Matches Played",
				"Avg xG For", "Avg xG Against", "Avg Goals For", "Avg Goals Against",
				"Finishing Delta", "Conceded Delta"}
		} else {
			headers = []string{"Period", "Country", "League", "Team", "Matches Played",
				"Avg xG For", "Avg xG Against", "xG Diff", "Avg Goals For", "Avg Goals Against",
				"Finishing Delta", "Conceded Delta"}
		}
	}
	if len(headers) > 0 {
		if err := writer.Write(headers); err != nil {
			return err
		}
	}

	for _, row := range rows {
		record := make([]string, len(row))
		for i, cell := range row {
			record[i] = fmt.Sprintf("%v", cell)
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("error escribiendo CSV: %w", err)
	}

	return nil
}

func buildPayload(tableID, category, nonce string, length int) string {
	combinedTables := map[string]bool{"85": true, "91": true, "44": true, "46": true, "7": true, "9": true}
	if combinedTables[tableID] {
		return buildPayloadCombined(tableID, category, nonce, length)
	}

	switch strings.ToLower(strings.TrimSpace(category)) {
	case "corners":
		return fmt.Sprintf(
			"action=get_wdtable&table_id=%s&draw=1&columns[0][data]=0&columns[0][name]=Team&columns[0][searchable]=true&columns[0][orderable]=true&columns[0][search][value]=&columns[0][search][regex]=false&columns[1][data]=1&columns[1][name]=TG&columns[1][searchable]=true&columns[1][orderable]=true&columns[1][search][value]=&columns[1][search][regex]=false&columns[2][data]=2&columns[2][name]=AVCFT&columns[2][searchable]=true&columns[2][orderable]=true&columns[2][search][value]=&columns[2][search][regex]=false&columns[3][data]=3&columns[3][name]=AVCRFT&columns[3][searchable]=true&columns[3][orderable]=true&columns[3][search][value]=&columns[3][search][regex]=false&columns[4][data]=4&columns[4][name]=O75CFT&columns[4][searchable]=true&columns[4][orderable]=true&columns[4][search][value]=&columns[4][search][regex]=false&columns[5][data]=5&columns[5][name]=U75CFT&columns[5][searchable]=true&columns[5][orderable]=true&columns[5][search][value]=&columns[5][search][regex]=false&columns[6][data]=6&columns[6][name]=O85CFT&columns[6][searchable]=true&columns[6][orderable]=true&columns[6][search][value]=&columns[6][search][regex]=false&columns[7][data]=7&columns[7][name]=U85CFT&columns[7][searchable]=true&columns[7][orderable]=true&columns[7][search][value]=&columns[7][search][regex]=false&columns[8][data]=8&columns[8][name]=O95CFT&columns[8][searchable]=true&columns[8][orderable]=true&columns[8][search][value]=&columns[8][search][regex]=false&columns[9][data]=9&columns[9][name]=U95CFT&columns[9][searchable]=true&columns[9][orderable]=true&columns[9][search][value]=&columns[9][search][regex]=false&columns[10][data]=10&columns[10][name]=O105CFT&columns[10][searchable]=true&columns[10][orderable]=true&columns[10][search][value]=&columns[10][search][regex]=false&columns[11][data]=11&columns[11][name]=U105CFT&columns[11][searchable]=true&columns[11][orderable]=true&columns[11][search][value]=&columns[11][search][regex]=false&order[0][column]=2&order[0][dir]=desc&start=0&length=%d&search[value]=&search[regex]=false&wdtNonce=%s",
			tableID, length, nonce,
		)
	case "btts":
		return fmt.Sprintf(
			"action=get_wdtable&table_id=%s&draw=1&columns[0][data]=0&columns[0][name]=Team&columns[0][searchable]=true&columns[0][orderable]=true&columns[0][search][value]=&columns[0][search][regex]=false&columns[1][data]=1&columns[1][name]=TGA&columns[1][searchable]=true&columns[1][orderable]=true&columns[1][search][value]=&columns[1][search][regex]=false&columns[2][data]=2&columns[2][name]=BTTSYA1H&columns[2][searchable]=true&columns[2][orderable]=true&columns[2][search][value]=&columns[2][search][regex]=false&columns[3][data]=3&columns[3][name]=BTTSNA1H&columns[3][searchable]=true&columns[3][orderable]=true&columns[3][search][value]=&columns[3][search][regex]=false&order[0][column]=2&order[0][dir]=desc&start=0&length=%d&search[value]=&search[regex]=false&wdtNonce=%s",
			tableID, length, nonce,
		)
	case "wdl":
		return fmt.Sprintf(
			"action=get_wdtable&table_id=%s&draw=1&columns[0][data]=0&columns[0][name]=Team&columns[0][searchable]=true&columns[0][orderable]=true&columns[0][search][value]=&columns[0][search][regex]=false&columns[1][data]=1&columns[1][name]=TGA&columns[1][searchable]=true&columns[1][orderable]=true&columns[1][search][value]=&columns[1][search][regex]=false&columns[2][data]=2&columns[2][name]=WAFT&columns[2][searchable]=true&columns[2][orderable]=true&columns[2][search][value]=&columns[2][search][regex]=false&columns[3][data]=3&columns[3][name]=DAFT&columns[3][searchable]=true&columns[3][orderable]=true&columns[3][search][value]=&columns[3][search][regex]=false&columns[4][data]=4&columns[4][name]=LAFT&columns[4][searchable]=true&columns[4][orderable]=true&columns[4][search][value]=&columns[4][search][regex]=false&order[0][column]=2&order[0][dir]=desc&start=0&length=%d&search[value]=&search[regex]=false&wdtNonce=%s",
			tableID, length, nonce,
		)
	case "xg":
		return fmt.Sprintf(
			"action=get_wdtable&table_id=%s&draw=1&columns[0][data]=0&columns[0][name]=TimeFrame&columns[0][searchable]=true&columns[0][orderable]=true&columns[0][search][value]=&columns[0][search][regex]=false&columns[1][data]=1&columns[1][name]=Country&columns[1][searchable]=true&columns[1][orderable]=true&columns[1][search][value]=&columns[1][search][regex]=false&columns[2][data]=2&columns[2][name]=League&columns[2][searchable]=true&columns[2][orderable]=true&columns[2][search][value]=&columns[2][search][regex]=false&columns[3][data]=3&columns[3][name]=Team&columns[3][searchable]=true&columns[3][orderable]=true&columns[3][search][value]=&columns[3][search][regex]=false&columns[4][data]=4&columns[4][name]=Matches Played&columns[4][searchable]=true&columns[4][orderable]=true&columns[4][search][value]=&columns[4][search][regex]=false&columns[5][data]=5&columns[5][name]=Avg xG For&columns[5][searchable]=true&columns[5][orderable]=true&columns[5][search][value]=&columns[5][search][regex]=false&columns[6][data]=6&columns[6][name]=Avg xG Against&columns[6][searchable]=true&columns[6][orderable]=true&columns[6][search][value]=&columns[6][search][regex]=false&columns[7][data]=7&columns[7][name]=xG Diff&columns[7][searchable]=true&columns[7][orderable]=true&columns[7][search][value]=&columns[7][search][regex]=false&columns[8][data]=8&columns[8][name]=Avg Goals For&columns[8][searchable]=true&columns[8][orderable]=true&columns[8][search][value]=&columns[8][search][regex]=false&columns[9][data]=9&columns[9][name]=Avg Goals Against&columns[9][searchable]=true&columns[9][orderable]=true&columns[9][search][value]=&columns[9][search][regex]=false&columns[10][data]=10&columns[10][name]=Finishing Delta&columns[10][searchable]=true&columns[10][orderable]=true&columns[10][search][value]=&columns[10][search][regex]=false&columns[11][data]=11&columns[11][name]=Conceded Delta&columns[11][searchable]=true&columns[11][orderable]=true&columns[11][search][value]=&columns[11][search][regex]=false&order[0][column]=5&order[0][dir]=desc&start=0&length=%d&search[value]=&search[regex]=false&wdtNonce=%s",
			tableID, length, nonce,
		)
	default:
		return fmt.Sprintf("action=get_wdtable&table_id=%s&draw=1&start=0&length=%d&search[value]=&search[regex]=false&wdtNonce=%s", tableID, length, nonce)
	}
}

// buildPayloadCombined genera el payload de 8/16/9 columnas para tablas combinadas globales (85, 91, 44, 46, 7, 9):
// [0]=TimeFrame, [1]=CompetitionType, [2]=Country, [3]=League, [4]=Team, [5]=TG/TGA, [6..]=métricas.
func buildPayloadCombined(tableID, category, nonce string, length int) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "corners":
		// 16 columnas: TimeFrame(0), Type(1), Country(2), League(3), Team(4), TG(5), AVCFT(6)..U105CFT(15)
		return fmt.Sprintf(
			"action=get_wdtable&table_id=%s&draw=1&columns[0][data]=0&columns[0][name]=TimeFrame&columns[0][searchable]=true&columns[0][orderable]=true&columns[0][search][value]=&columns[0][search][regex]=false&columns[1][data]=1&columns[1][name]=CompetitionType&columns[1][searchable]=true&columns[1][orderable]=true&columns[1][search][value]=&columns[1][search][regex]=false&columns[2][data]=2&columns[2][name]=Country&columns[2][searchable]=true&columns[2][orderable]=true&columns[2][search][value]=&columns[2][search][regex]=false&columns[3][data]=3&columns[3][name]=League&columns[3][searchable]=true&columns[3][orderable]=true&columns[3][search][value]=&columns[3][search][regex]=false&columns[4][data]=4&columns[4][name]=Team&columns[4][searchable]=true&columns[4][orderable]=true&columns[4][search][value]=&columns[4][search][regex]=false&columns[5][data]=5&columns[5][name]=TG&columns[5][searchable]=true&columns[5][orderable]=true&columns[5][search][value]=&columns[5][search][regex]=false&columns[6][data]=6&columns[6][name]=AVCFT&columns[6][searchable]=true&columns[6][orderable]=true&columns[6][search][value]=&columns[6][search][regex]=false&columns[7][data]=7&columns[7][name]=AVCRFT&columns[7][searchable]=true&columns[7][orderable]=true&columns[7][search][value]=&columns[7][search][regex]=false&columns[8][data]=8&columns[8][name]=O75CFT&columns[8][searchable]=true&columns[8][orderable]=true&columns[8][search][value]=&columns[8][search][regex]=false&columns[9][data]=9&columns[9][name]=U75CFT&columns[9][searchable]=true&columns[9][orderable]=true&columns[9][search][value]=&columns[9][search][regex]=false&columns[10][data]=10&columns[10][name]=O85CFT&columns[10][searchable]=true&columns[10][orderable]=true&columns[10][search][value]=&columns[10][search][regex]=false&columns[11][data]=11&columns[11][name]=U85CFT&columns[11][searchable]=true&columns[11][orderable]=true&columns[11][search][value]=&columns[11][search][regex]=false&columns[12][data]=12&columns[12][name]=O95CFT&columns[12][searchable]=true&columns[12][orderable]=true&columns[12][search][value]=&columns[12][search][regex]=false&columns[13][data]=13&columns[13][name]=U95CFT&columns[13][searchable]=true&columns[13][orderable]=true&columns[13][search][value]=&columns[13][search][regex]=false&columns[14][data]=14&columns[14][name]=O105CFT&columns[14][searchable]=true&columns[14][orderable]=true&columns[14][search][value]=&columns[14][search][regex]=false&columns[15][data]=15&columns[15][name]=U105CFT&columns[15][searchable]=true&columns[15][orderable]=true&columns[15][search][value]=&columns[15][search][regex]=false&order[0][column]=6&order[0][dir]=desc&start=0&length=%d&search[value]=&search[regex]=false&wdtNonce=%s",
			tableID, length, nonce,
		)
	case "btts":
		// 8 columnas: TimeFrame(0), Type(1), Country(2), League(3), Team(4), TGA(5), BTTSYA1H(6), BTTSNA1H(7)
		return fmt.Sprintf(
			"action=get_wdtable&table_id=%s&draw=1&columns[0][data]=0&columns[0][name]=TimeFrame&columns[0][searchable]=true&columns[0][orderable]=true&columns[0][search][value]=&columns[0][search][regex]=false&columns[1][data]=1&columns[1][name]=CompetitionType&columns[1][searchable]=true&columns[1][orderable]=true&columns[1][search][value]=&columns[1][search][regex]=false&columns[2][data]=2&columns[2][name]=Country&columns[2][searchable]=true&columns[2][orderable]=true&columns[2][search][value]=&columns[2][search][regex]=false&columns[3][data]=3&columns[3][name]=League&columns[3][searchable]=true&columns[3][orderable]=true&columns[3][search][value]=&columns[3][search][regex]=false&columns[4][data]=4&columns[4][name]=Team&columns[4][searchable]=true&columns[4][orderable]=true&columns[4][search][value]=&columns[4][search][regex]=false&columns[5][data]=5&columns[5][name]=TGA&columns[5][searchable]=true&columns[5][orderable]=true&columns[5][search][value]=&columns[5][search][regex]=false&columns[6][data]=6&columns[6][name]=BTTSYA1H&columns[6][searchable]=true&columns[6][orderable]=true&columns[6][search][value]=&columns[6][search][regex]=false&columns[7][data]=7&columns[7][name]=BTTSNA1H&columns[7][searchable]=true&columns[7][orderable]=true&columns[7][search][value]=&columns[7][search][regex]=false&order[0][column]=6&order[0][dir]=desc&start=0&length=%d&search[value]=&search[regex]=false&wdtNonce=%s",
			tableID, length, nonce,
		)
	case "wdl":
		// 9 columnas: TimeFrame(0), Type(1), Country(2), League(3), Team(4), TGA(5), WAFT(6), DAFT(7), LAFT(8)
		return fmt.Sprintf(
			"action=get_wdtable&table_id=%s&draw=1&columns[0][data]=0&columns[0][name]=TimeFrame&columns[0][searchable]=true&columns[0][orderable]=true&columns[0][search][value]=&columns[0][search][regex]=false&columns[1][data]=1&columns[1][name]=CompetitionType&columns[1][searchable]=true&columns[1][orderable]=true&columns[1][search][value]=&columns[1][search][regex]=false&columns[2][data]=2&columns[2][name]=Country&columns[2][searchable]=true&columns[2][orderable]=true&columns[2][search][value]=&columns[2][search][regex]=false&columns[3][data]=3&columns[3][name]=League&columns[3][searchable]=true&columns[3][orderable]=true&columns[3][search][value]=&columns[3][search][regex]=false&columns[4][data]=4&columns[4][name]=Team&columns[4][searchable]=true&columns[4][orderable]=true&columns[4][search][value]=&columns[4][search][regex]=false&columns[5][data]=5&columns[5][name]=TGA&columns[5][searchable]=true&columns[5][orderable]=true&columns[5][search][value]=&columns[5][search][regex]=false&columns[6][data]=6&columns[6][name]=WAFT&columns[6][searchable]=true&columns[6][orderable]=true&columns[6][search][value]=&columns[6][search][regex]=false&columns[7][data]=7&columns[7][name]=DAFT&columns[7][searchable]=true&columns[7][orderable]=true&columns[7][search][value]=&columns[7][search][regex]=false&columns[8][data]=8&columns[8][name]=LAFT&columns[8][searchable]=true&columns[8][orderable]=true&columns[8][search][value]=&columns[8][search][regex]=false&order[0][column]=6&order[0][dir]=desc&start=0&length=%d&search[value]=&search[regex]=false&wdtNonce=%s",
			tableID, length, nonce,
		)
	default:
		return fmt.Sprintf("action=get_wdtable&table_id=%s&draw=1&start=0&length=%d&search[value]=&search[regex]=false&wdtNonce=%s", tableID, length, nonce)
	}
}

func decompressBody(resp *http.Response, body []byte) ([]byte, error) {
	encoding := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	switch encoding {
	case "zstd":
		decoder, err := zstd.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer decoder.Close()
		return io.ReadAll(decoder)
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	case "deflate":
		reader := flate.NewReader(bytes.NewReader(body))
		defer reader.Close()
		return io.ReadAll(reader)
	default:
		return body, nil
	}
}
