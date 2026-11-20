package main

import (
	"fmt"
	"futcor_predict/models"
	"futcor_predict/ui"
	"log"
	"os"
	"path/filepath"
	"runtime" // Importante para detectar Android
	"time"
)

func resolveDataDir() string {
	// 1. Si estamos en Android, usar la ruta absoluta (requiere MANAGE_EXTERNAL_STORAGE)
	if runtime.GOOS == "android" {
		ruta := "/storage/emulated/0/futcor/data"
		// Asegurarnos de que la carpeta existe
		if err := os.MkdirAll(ruta, 0755); err != nil {
			log.Printf("⚠️ No se pudo crear la carpeta en Android: %v", err)
		}
		return ruta
	}

	// 2. Si estamos en PC (Windows/Linux/Mac)
	if info, err := os.Stat("data"); err == nil && info.IsDir() {
		return "data"
	}
	
	// 3. Si no, buscar al lado del ejecutable
	if exePath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exePath), "data")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	
	return "data"
}

func main() {
	predictor := models.NewPredictor()
	dataDir := resolveDataDir()

	fmt.Println("🚀 Iniciando Futbol Predictor...")
	fmt.Println("📂 Usando directorio de datos:", dataDir)

	// 1. Cargar equipos base
	err := predictor.LoadTeamsFromCSV(filepath.Join(dataDir, "equipos.csv"))
	if err != nil {
		log.Printf("⚠️ Advertencia: No se pudieron cargar los equipos: %v", err)
		log.Println("📋 Usando datos de ejemplo...")
		createSampleData(predictor)
	} else {
		fmt.Printf("✅ %d equipos cargados\n", len(predictor.Teams))
	}

	// 2. Cargar estadísticas por liga
	ligas := []string{
		"Spain LaLiga",
		"England Premier League",
		"Germany Bundesliga",
		"Italy Serie A",
		"France Ligue 1",
		"Brazil Brazilian League",
		"Mexico Liga MX",
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
	}

	// 3. Cargar corners
	fmt.Println("📐 Cargando estadísticas de corners...")
	predictor.LoadCornersData(filepath.Join(dataDir, "team_Corners_FT_Home_Spain LaLiga.csv"), "home")
	predictor.LoadCornersData(filepath.Join(dataDir, "team_Corners_FT_Away_Spain LaLiga.csv"), "away")

	fmt.Println("✨ Iniciando interfaz gráfica...")
	time.Sleep(300 * time.Millisecond)

	// PASAMOS EL dataDir A LA INTERFAZ
	fyneUI := ui.NewFyneUI(predictor, dataDir)
	fyneUI.Run()
}

func createSampleData(predictor *models.Predictor) {
	// ... (deja tu función createSampleData exactamente como estaba) ...
	// Si no la tienes a mano, puedo volver a pasártela, pero es la misma de antes.
}
