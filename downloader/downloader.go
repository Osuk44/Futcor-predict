package downloader

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"gopkg.in/yaml.v3"
)

type TableConfig struct {
	Name         string `yaml:"name"`
	TableID      string `yaml:"table_id"`
	Referer      string `yaml:"referer"`
	Category     string `yaml:"category"`
	League       string `yaml:"league"`
	FilterLeague string `yaml:"filter_league"`
	KeepColumns  []int  `yaml:"keep_columns"`
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

var commonHeaders = map[string]string{
	"accept":           "application/json, text/javascript, */*; q=0.01",
	"accept-encoding":  "gzip, deflate, zstd",
	"accept-language":  "es-419,es;q=0.9",
	"content-type":     "application/x-www-form-urlencoded; charset=UTF-8",
	"origin":           "https://footiqo.com",
	"user-agent":       "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
	"x-requested-with": "XMLHttpRequest",
}

var (
	reNonceNameFirst  = regexp.MustCompile(`(?i)name="(wdtNonce[^"]*)"[^>]*value="([^"]+)"`)
	reNonceValueFirst = regexp.MustCompile(`(?i)value="([^"]+)"[^>]*name="(wdtNonce[^"]*)"`)
)

func fetchPageNonces(client *http.Client, referer string) (*pageNonces, error) {
	req, err := http.NewRequest("GET", referer, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", commonHeaders["user-agent"])
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Encoding", commonHeaders["accept-encoding"])
	req.Header.Set("Accept-Language", commonHeaders["accept-language"])

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d al obtener HTML de %s", resp.StatusCode, referer)
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
	client := &http.Client{Jar: jar, Timeout: 60 * time.Second}

	nonceCache := make(map[string]*pageNonces)
	rawTableCache := make(map[string][][]interface{})
	exitos := 0
	errores := 0
	var ultimoError error

	for i, cfg := range tables {
		fmt.Printf("[%d/%d] 📥 %s\n", i+1, len(tables), cfg.Name)

		rows, fromCache, err := getTableRowsWithRetry(client, cfg, nonceCache, rawTableCache)
		if err != nil {
			fmt.Printf("  ❌ Error: %v\n", err)
			errores++
			ultimoError = err
			if !fromCache {
				time.Sleep(1500 * time.Millisecond)
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
			time.Sleep(1200 * time.Millisecond)
		}
	}

	if exitos == 0 {
		return fmt.Errorf("fallaron todas las descargas. Último error: %v", ultimoError)
	}
	if errores > 0 {
		return fmt.Errorf("descarga parcial: %d exitosas, %d fallidas (Último error: %v)", exitos, errores, ultimoError)
	}

	return nil
}

func getTableRowsWithRetry(
	client *http.Client,
	cfg TableConfig,
	nonceCache map[string]*pageNonces,
	rawTableCache map[string][][]interface{},
) ([][]interface{}, bool, error) {
	if cachedRows, ok := rawTableCache[cfg.TableID]; ok {
		return cachedRows, true, nil
	}

	const maxAttempts = 3
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			// Invalidar nonce en reintento y esperar con backoff progresivo
			delete(nonceCache, cfg.Referer)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		nonces, exists := nonceCache[cfg.Referer]
		if !exists {
			var err error
			nonces, err = fetchPageNonces(client, cfg.Referer)
			if err != nil {
				lastErr = fmt.Errorf("error obteniendo nonce: %w", err)
				continue
			}
			nonceCache[cfg.Referer] = nonces
			time.Sleep(800 * time.Millisecond)
		}

		nonce := nonces.getForTable(cfg.TableID)
		rows, err := fetchTableRows(client, cfg, nonce)
		if err != nil {
			lastErr = err
			continue
		}

		rawTableCache[cfg.TableID] = rows
		return rows, false, nil
	}

	return nil, false, lastErr
}

func fetchTableRows(client *http.Client, cfg TableConfig, nonce string) ([][]interface{}, error) {
	url := fmt.Sprintf("https://footiqo.com/wp-admin/admin-ajax.php?action=get_wdtable&table_id=%s", cfg.TableID)
	payload := buildPayload(cfg.TableID, cfg.Category, nonce, 2000)

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(payload))
	if err != nil {
		return nil, err
	}

	for k, v := range commonHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("Referer", cfg.Referer)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	body, err := decompressBody(resp, rawBody)
	if err != nil {
		return nil, fmt.Errorf("error descomprimiendo: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d en %s", resp.StatusCode, cfg.Name)
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
		var filtered [][]interface{}
		for _, row := range rows {
			if len(row) < 3 {
				continue
			}
			league, ok := row[2].(string)
			if ok && league == cfg.FilterLeague {
				filtered = append(filtered, row)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("no se encontraron filas con liga '%s'", cfg.FilterLeague)
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
