package main

import (
	"fmt"
	"futcor_predict/models"
	"futcor_predict/ui"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Variable global para saber si tenemos permiso de escritura
var hasStoragePermission = true

func resolveDataDir() string {
	if runtime.GOOS == "android" {
		ruta := "/storage/emulated/0/futcor/data"
		
		// Intentar crear la carpeta
		err := os.MkdirAll(ruta, 0755)
		if err != nil {
			log.Printf("❌ ERROR: Sin permisos para crear %s: %v", ruta, err)
			hasStoragePermission = false
			
			// Fallback a la carpeta privada de la app para que no crashee
			fallback, _ := os.UserConfigDir()
			return filepath.Join(fallback, "data")
		}
		
		// Verificar si realmente podemos escribir un archivo de prueba
		testFile := filepath.Join(ruta, ".test_write")
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			log.Printf("❌ ERROR: Carpeta creada pero sin permiso de escritura: %v", err)
			hasStoragePermission = false
			fallback, _ := os.UserConfigDir()
			return filepath.Join(fallback, "data")
		}
		os.Remove(testFile) // Limpiamos el archivo de prueba
		
		return ruta
	}

	// Código para PC (Windows/Linux/Mac)
	if info, err := os.Stat("data"); err == nil && info.IsDir() {
		return "data"
	}
	if exePath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exePath), "data")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return "data"
}

func main() {
	// 1. Resolver el directorio de datos
	dataDir := resolveDataDir()
	
	// 2. Si estamos en Android y no hay permiso, abrir los ajustes automáticamente
	if runtime.GOOS == "android" && !hasStoragePermission {
		log.Println("⚠️ Abriendo ajustes de Android para solicitar permiso...")
		// Este comando de Android abre la pantalla de permisos de la app
		cmd := exec.Command("am", "start", "-a", "android.settings.MANAGE_APP_ALL_FILES_ACCESS_PERMISSION", "-d", "package:com.futcor.predictor")
		cmd.Run()
	}

	predictor := models.NewPredictor()

	fmt.Println("🚀 Iniciando Futbol Predictor...")
	fmt.Println("📂 Usando directorio de datos:", dataDir)

	// 3. Cargar equipos base
	err := predictor.LoadTeamsFromCSV(filepath.Join(dataDir, "equipos.csv"))
	if err != nil {
		log.Printf("⚠️ Advertencia: No se pudieron cargar los equipos: %v", err)
		log.Println("📋 Usando datos de ejemplo...")
		createSampleData(predictor)
	} else {
		fmt.Printf("✅ %d equipos cargados\n", len(predictor.Teams))
	}

	// 4. Cargar estadísticas por liga (BTTS, xG, WDL y CÓRNERS)
	ligas := []string{
		"Spain LaLiga",
		"England Premier League",
		"Germany Bundesliga",
		"Italy Serie A",
		"France Ligue 1",
		"Brazil Brazilian League",
		"Mexico Liga MX",
		"Europe Champions League", // Añadida Champions que tenías en el YAML
	}

	fmt.Println("📈 Cargando estadísticas de ligas...")

	for _, liga := range ligas {
		predictor.LoadBTTSData(filepath.Join(dataDir, fmt.Sprintf("team_BTTS_1H_Overall_%s.csv", liga)), "1H")
		predictor.LoadBTTSData(filepath.Join(dataDir, fmt.Sprintf("team_BTTS_2H_Overall_%s.csv", liga)), "2H")

		predictor.LoadXGSeparatedData(
			filepath.Join(dataDir, fmt.Sprintf("team_xG_FT_Home_%s.csv", liga)),
			filepath.Join(dataDir, fmt.Sprintf("team_xG_FT_Away_%s.csv", liga)),
		)

		predictor.LoadWDLData(filepath.Join(dataDir, fmt.Sprintf("team_WDL_FT_Home_%s.csv", liga)), "home")
		predictor.LoadWDLData(filepath.Join(dataDir, fmt.Sprintf("team_WDL_FT_Away_%s.csv", liga)), "away")
		
		// CORRECCIÓN: Cargar córners de TODAS las ligas, no solo LaLiga
		predictor.LoadCornersData(filepath.Join(dataDir, fmt.Sprintf("team_Corners_FT_Home_%s.csv", liga)), "home")
		predictor.LoadCornersData(filepath.Join(dataDir, fmt.Sprintf("team_Corners_FT_Away_%s.csv", liga)), "away")
	}

	fmt.Println("✨ Iniciando interfaz gráfica...")
	time.Sleep(300 * time.Millisecond)

	// 5. Pasar dataDir y el estado del permiso a la UI
	fyneUI := ui.NewFyneUI(predictor, dataDir)
	
	// Si no hay permiso, mostraremos un diálogo en la UI (necesitamos añadir esta función a fut_fyne.go)
	if runtime.GOOS == "android" && !hasStoragePermission {
		fyneUI.MostrarAlertaPermiso = true
	}
	
	fyneUI.Run()
}

func createSampleData(predictor *models.Predictor) {
	// ... (Mantén tu código de createSampleData igual)
}
