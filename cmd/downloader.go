package main

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	//"regexp"
	//"strings"
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

var commonHeaders = map[string]string{
	"accept":             "application/json, text/javascript, */*; q=0.01",
	"accept-encoding":    "gzip, deflate, br, zstd",
	"accept-language":    "es-419,es;q=0.9",
	"content-type":       "application/x-www-form-urlencoded; charset=UTF-8",
	"origin":             "https://footiqo.com",
	"priority":           "u=1, i",
	"sec-ch-ua":          `"Not(A:Brand";v="8", "Chromium";v="144", "Brave";v="144"`,
	"sec-ch-ua-mobile":   "?1",
	"sec-ch-ua-platform": `"Android"`,
	"sec-fetch-dest":     "empty",
	"sec-fetch-mode":     "cors",
	"sec-fetch-site":     "same-origin",
	"sec-gpc":            "1",
	"user-agent":         "Mozilla/5.0 (Linux; Android 6.0; Nexus 5 Build/MRA58N) AppleWebKit/537.36 (Chrome/144.0.0.0 Mobile Safari/537.36)",
	"x-requested-with":   "XMLHttpRequest",
}

const cookie = "xxxx"

// noncesPorTabla - Todos los nonces capturados (actualizados al 17/04/2026)
var noncesPorTabla = map[string]string{
    // ==================== BTTS ====================
    "385": "bd0e43f18c", // LaLiga 1H
    "388": "5978dd4abb", // LaLiga 2H
    "523": "fcd2635eb8", // Francia 1H
    "526": "aa7f405fd6", // Francia 2H
    "340": "a12d7213e7", // Premier 1H
    "343": "37088ae740", // Premier 2H
    "432": "1e2ce8701f", // Bundesliga 1H
    "435": "2623aabd9a", // Bundesliga 2H
    "477": "999d472bff", // Serie A 1H
    "480": "79a719fec6", // Serie A 2H
    "622": "7d8387ff02", // Champions 1H
    "625": "f65cd1d141", // Champions 2H
    "85":  "e0c1acd46f", // Tabla global BTTS 1H (México, Brasil)
    "91":  "2b24763a79", // Tabla global BTTS 2H (México, Brasil)

    // ==================== Corners ====================
    "602": "1d09b71e74", // Champions 1H
    "603": "9b21d5c59a", // Champions 2H
    "412": "14f5027af0", // Bundesliga 1H
    "413": "2978ba0566", // Bundesliga 2H
    "365": "b95541a716", // LaLiga 1H
    "366": "2d2ff6a12f", // LaLiga 2H
    "457": "1e141fb359", // Serie A 1H
    "458": "e60f4d5d5e", // Serie A 2H
    "320": "a7989a6395", // Premier 1H
    "321": "066e72e8c4", // Premier 2H
    "502": "f13bd6f709", // Ligue 1 1H
    "503": "5b93eb86f5", // Ligue 1 2H
    "44":  "7a581de083", // Tabla global corners 1H (México, Brasil)
    "46":  "81758ab4a3", // Tabla global corners 2H (México, Brasil)

    // ==================== WDL ====================
    "394": "fa304bd5be", // Bundesliga 1H
    "395": "39d816fcf9", // Bundesliga 2H
    "347": "0d9ea47f9e", // LaLiga 1H
    "348": "d5be7c3bf2", // LaLiga 2H
    "439": "d978798fc5", // Serie A 1H
    "440": "e66fe7f649", // Serie A 2H
    "302": "31ceb505fa", // Premier 1H
    "303": "26a316a22d", // Premier 2H
    "484": "1dd026e7a1", // Ligue 1 1H
    "485": "69630bf4f1", // Ligue 1 2H
    "629": "3626f6c4f3", // Champions 1H
    "630": "8e95db4464", // Champions 2H
    "7":   "59c6d81885", // Tabla global WDL 1H (México, Brasil)
    "9":   "9f4969aa5e", // Tabla global WDL 2H (México, Brasil)

    // ==================== xG ====================
    "862": "a22e236dd6", // Champions 1H
    "863": "214c9c19aa", // Champions 2H
}

// getNonce devuelve el nonce del mapa
func getNonce(tableID string) (string, error) {
	if nonce, ok := noncesPorTabla[tableID]; ok && nonce != "" {
		return nonce, nil
	}
	return "", fmt.Errorf("nonce no definido para table_id=%s. Cópialo manualmente desde DevTools", tableID)
}

// construye payload según categoría, usando el nonce extraído dinámicamente
func buildPayload(tableID, category, nonce string, length int) string {
	switch category {
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

// descomprime el cuerpo según content-encoding
func decompressBody(resp *http.Response, body []byte) ([]byte, error) {
	encoding := resp.Header.Get("Content-Encoding")
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
	case "br":
		// Brotli no es común, pero si aparece habría que añadir librería
		return nil, fmt.Errorf("brotli no soportado")
	default:
		return body, nil
	}
}

func main() {
	yamlFile, err := os.ReadFile("tables.yaml")
	if err != nil {
		fmt.Printf("❌ Error leyendo tables.yaml: %v\n", err)
		return
	}

	var tables []TableConfig
	err = yaml.Unmarshal(yamlFile, &tables)
	if err != nil {
		fmt.Printf("❌ Error parseando YAML: %v\n", err)
		return
	}

	fmt.Printf("📋 Cargadas %d tablas para descargar\n\n", len(tables))

	outDir := "data"
	os.MkdirAll(outDir, 0755)

	for i, cfg := range tables {
		fmt.Printf("[%d/%d] 📥 Descargando: %s (table_id=%s, cat=%s)\n", i+1, len(tables), cfg.Name, cfg.TableID, cfg.Category)

		nonce, err := getNonce(cfg.TableID)
		if err != nil {
			fmt.Printf("  ❌ %v\n", err)
			continue
		}

		err = downloadTable(cfg, outDir, nonce)
		if err != nil {
			fmt.Printf("  ❌ Error: %v\n", err)
		} else {
			fmt.Printf("  ✅ Guardado: %s.csv\n", cfg.Name)
		}
		fmt.Println()
		time.Sleep(2 * time.Second)
	}
}

func downloadTable(cfg TableConfig, outDir, nonce string) error {
	url := fmt.Sprintf("https://footiqo.com/wp-admin/admin-ajax.php?action=get_wdtable&table_id=%s", cfg.TableID)
	payload := buildPayload(cfg.TableID, cfg.Category, nonce, 2000)

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(payload))
	if err != nil {
		return err
	}

	for k, v := range commonHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Referer", cfg.Referer)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(payload)))

	client := &http.Client{Timeout: 30 * time.Second}
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
		preview := string(body)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return fmt.Errorf("HTTP %d - %s", resp.StatusCode, preview)
	}

	if len(body) == 0 {
		return fmt.Errorf("respuesta vacía (posiblemente payload no válido)")
	}

	var result ResponseData
	if err := json.Unmarshal(body, &result); err != nil {
		preview := string(body)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return fmt.Errorf("error JSON: %v - Respuesta: %s", err, preview)
	}

	if len(result.Data) == 0 {
		return fmt.Errorf("no hay datos en la respuesta")
	}

	rows := result.Data

	// Filtrar por liga si se especificó
	if cfg.FilterLeague != "" {
		var filtered [][]interface{}
		for _, row := range rows {
			if len(row) < 3 {
				continue
			}
			league, ok := row[2].(string)
			if !ok {
				continue
			}
			if league == cfg.FilterLeague {
				filtered = append(filtered, row)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("no se encontraron filas con liga '%s'", cfg.FilterLeague)
		}
		rows = filtered
		fmt.Printf("  🎯 Filtrado: %d filas de la liga '%s'\n", len(rows), cfg.FilterLeague)
	}

		// Seleccionar columnas
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
    // Filtrar solo Current season para xG
if cfg.Category == "xg" {
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
        fmt.Printf("  🎯 Filtrado solo 'Current season': %d filas\n", len(rows))
    }
}
	// Crear archivo CSV
	outputPath := filepath.Join(outDir, cfg.Name+".csv")
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Escribir cabeceras según categoría (una sola vez)
	var headers []string
	switch cfg.Category {
	case "btts":
		headers = []string{"Team", "Matches Played", "BTTS YES %", "BTTS NO %"}
	case "corners":
		headers = []string{"Team", "Matches Played", "Avg Corners Taken", "Avg Corners Conceded",
			"Over 7.5 (%)", "Under 7.5 (%)", "Over 8.5 (%)", "Under 8.5 (%)",
			"Over 9.5 (%)", "Under 9.5 (%)", "Over 10.5 (%)", "Under 10.5 (%)"}
	case "wdl":
		headers = []string{"Team", "Matches Played", "Win %", "Draw %", "Loss %"}
	case "xg":
		headers = []string{"Period", "Country", "League", "Team", "Matches Played",
			"Avg xG For", "Avg xG Against", "xG Diff", "Avg Goals For", "Avg Goals Against",
			"Finishing Delta", "Conceded Delta"}
	}
	if len(headers) > 0 {
		if err := writer.Write(headers); err != nil {
			return err
		}
	}

	// Escribir datos
	for _, row := range rows {
		record := make([]string, len(row))
		for i, cell := range row {
			record[i] = fmt.Sprintf("%v", cell)
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	fmt.Printf("  📊 %d filas guardadas\n", len(rows))
	return nil
}

