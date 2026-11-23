package downloader

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestLiveDownload(t *testing.T) {
	// Usa "../data" si quieres probar contra el tables.yaml actual
	err := DescargarTodo("../data")
	fmt.Printf("Resultado DescargarTodo: %v\n", err)
}

func TestInspectSpecificTables(t *testing.T) {
	outDir := "../data"
	yamlBytes, err := os.ReadFile(outDir + "/tables.yaml")
	if err != nil {
		t.Fatalf("error leyendo tables.yaml: %v", err)
	}

	var tables []TableConfig
	if err := yaml.Unmarshal(yamlBytes, &tables); err != nil {
		t.Fatalf("error parseando tables.yaml: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 60 * time.Second}
	nonceCache := make(map[string]*pageNonces)
	rawCache := make(map[string][][]interface{})
	failedReferers := make(map[string]error)

	for _, cfg := range tables {
		if cfg.TableID == "85" || cfg.TableID == "44" || cfg.TableID == "7" {
			fmt.Printf("Probando %s (table_id=%s)...\n", cfg.Name, cfg.TableID)
			rows, fromCache, err := getTableRowsWithRetry(client, cfg, nonceCache, rawCache, failedReferers)
			fmt.Printf("Resultado [%s]: err=%v | cache=%v | filas=%d\n", cfg.TableID, err, fromCache, len(rows))
		}
	}
}
