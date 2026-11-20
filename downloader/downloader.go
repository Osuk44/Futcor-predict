package downloader

import (
	"bytes"
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

// (Mantén tus estructuras TableConfig y ResponseData tal cual)
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

// MEJORA 1: Quitamos "br" y "deflate" para evitar fallos de descompresión
var commonHeaders = map[string]string{
	"accept":             "application/json, text/javascript, */*; q=0.01",
	"accept-encoding":    "gzip, zstd",
	"accept-language":    "es-419,es;q=0.9",
	"content-type":       "application/x-www-form-urlencoded; charset=UTF-8",
	"origin":             "https://footiqo.com",
	"user-agent":         "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
	"x-requested-with":   "XMLHttpRequest",
}

// MEJORA 2 y 3: Manejo de errores riguroso y Regex más amplia
func getDynamicNonce(client *http.Client, referer string) (string, error) {
	req, err := http.NewRequest("GET", referer, nil)
	if err != nil {
		return "", err
	}

	// No forzamos Accept-Encoding aquí, dejamos que Go maneje el gzip nativo si es posible
	req.Header.Set("User-Agent", commonHeaders["user-agent"])
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d al obtener HTML de %s", resp.StatusCode, referer)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error leyendo HTML: %w", err)
	}

	body, err := decompressBody(resp, bodyBytes)
	if err != nil {
		return "", fmt.Errorf("error descomprimiendo HTML: %w", err)
	}

	html := string(body)

	// Regex mejorada: busca wdtNonce o wdt_nonce, con comillas simples, dobles o escapadas (\")
	re := regexp.MustCompile(`(?:wdtNonce|wdt_nonce)[\\"'\s]*[:=][\\"'\s]*([a-zA-Z0-9]+)`)
	matches := re.FindStringSubmatch(html)

	if len(matches) < 2 {
		return "", fmt.Errorf("no se encontró el nonce en %s (posible bloqueo o cambio de web)", referer)
	}

	return matches[1], nil
}

// MEJORA 4 y 5: Control de directorios, caché de nonces y conteo de errores
func DescargarTodo(outDir string) error {
	// 5. Asegurar que el directorio existe
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
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	// 3. Caché de nonces por URL (Referer) para no saturar la web
	nonceCache := make(map[string]string)

	exitos := 0
	errores := 0
	var ultimoError error

	fmt.Printf("📋 Procesando %d tablas...\n", len(tables))

	for i, cfg := range tables {
		fmt.Printf("[%d/%d] 📥 %s\n", i+1, len(tables), cfg.Name)

		nonce, exists := nonceCache[cfg.Referer]
		if !exists {
			var err error
			nonce, err = getDynamicNonce(client, cfg.Referer)
			if err != nil {
				fmt.Printf("  ❌ Error obteniendo nonce: %v\n", err)
				errores++
				ultimoError = err
				continue
			}
			nonceCache[cfg.Referer] = nonce
			// Pausa pequeña solo cuando sacamos un nonce nuevo para simular humano
			time.Sleep(1 * time.Second) 
		}

		err := downloadTable(client, cfg, outDir, nonce)
		if err != nil {
			fmt.Printf("  ❌ Error descargando: %v\n", err)
			errores++
			ultimoError = err
		} else {
			fmt.Printf("  ✅ Guardado\n")
			exitos++
		}
		time.Sleep(500 * time.Millisecond) // Pausa entre descargas
	}

	// 4. Retorno honesto para la UI
	if exitos == 0 {
		return fmt.Errorf("fallaron todas las descargas. Último error: %v", ultimoError)
	}
	if errores > 0 {
		return fmt.Errorf("descarga parcial: %d exitosas, %d fallidas (Último error: %v)", exitos, errores, ultimoError)
	}

	return nil
}

// downloadTable ahora recibe el cliente para mantener cookies
func downloadTable(client *http.Client, cfg TableConfig, outDir, nonce string) error {
	url := fmt.Sprintf("https://footiqo.com/wp-admin/admin-ajax.php?action=get_wdtable&table_id=%s", cfg.TableID)
	payload := buildPayload(cfg.TableID, cfg.Category, nonce, 2000)

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(payload))
	if err != nil {
		return err
	}

	for k, v := range commonHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("Referer", cfg.Referer)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(payload)))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	body, err := decompressBody(resp, rawBody)
	if err != nil {
		return fmt.Errorf("error descomprimiendo: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 100)]))
	}

	var result ResponseData
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("error JSON: %v", err)
	}

	if len(result.Data) == 0 {
		return fmt.Errorf("no hay datos (posible nonce inválido o sin partidos)")
	}

	// ... AQUÍ VA TU LÓGICA DE FILTRADO Y GUARDADO EN CSV ...
	// (Mantén exactamente igual tu código de filtrado de ligas, columnas y escritura de CSV)

	return nil // Retorna nil si se guardó bien
}

func decompressBody(resp *http.Response, body []byte) ([]byte, error) {
	encoding := resp.Header.Get("Content-Encoding")
	switch encoding {
	case "zstd":
		decoder, err := zstd.NewReader(bytes.NewReader(body))
		if err != nil { return nil, err }
		defer decoder.Close()
		return io.ReadAll(decoder)
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil { return nil, err }
		defer reader.Close()
		return io.ReadAll(reader)
	default:
		return body, nil
	}
}

// Auxiliar para no imprimir respuestas gigantes en errores
func min(a, b int) int {
	if a < b { return a }
	return b
}
