package main

import (
	"errors"
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

var hasStoragePermission = true

func resolveDataDir() string {
	if runtime.GOOS == "android" {
		ruta := "/storage/emulated/0/futcor/data"

		if err := os.MkdirAll(ruta, 0755); err != nil {
			log.Printf("❌ Sin permisos para crear %s: %v", ruta, err)
			hasStoragePermission = false
			return androidFallbackDir()
		}

		testFile := filepath.Join(ruta, ".test_write")
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			log.Printf("❌ Carpeta creada sin permiso de escritura: %v", err)
			hasStoragePermission = false
			return androidFallbackDir()
		}
		_ = os.Remove(testFile)
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

func androidFallbackDir() string {
	if cfgDir, err := os.UserConfigDir(); err == nil && cfgDir != "" {
		dir := filepath.Join(cfgDir, "futcor", "data")
		if mkErr := os.MkdirAll(dir, 0755); mkErr == nil {
			return dir
		}
	}
	dir := filepath.Join(os.TempDir(), "futcor", "data")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func logIfUnexpectedErr(label string, err error) {
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("⚠️ Error cargando %s: %v", label, err)
	}
}

func loadAllStats(predictor *models.Predictor, dataDir string) {
	ligas := []string{
		"Spain LaLiga",
		"England Premier League",
		"Germany Bundesliga",
		"Italy Serie A",
		"France Ligue 1",
		"Brazil Brazilian League",
		"Mexico Liga MX",
		"Europe Champions League",
	}

	for _, liga := range ligas {
		logIfUnexpectedErr("BTTS 1H "+liga,
			predictor.LoadBTTSData(filepath.Join(dataDir, fmt.Sprintf("team_BTTS_1H_Overall_%s.csv", liga)), "1H"))
		logIfUnexpectedErr("BTTS 2H "+liga,
			predictor.LoadBTTSData(filepath.Join(dataDir, fmt.Sprintf("team_BTTS_2H_Overall_%s.csv", liga)), "2H"))

		logIfUnexpectedErr("xG "+liga,
			predictor.LoadXGSeparatedData(
				filepath.Join(dataDir, fmt.Sprintf("team_xG_FT_Home_%s.csv", liga)),
				filepath.Join(dataDir, fmt.Sprintf("team_xG_FT_Away_%s.csv", liga)),
			))

		logIfUnexpectedErr("WDL Home "+liga,
			predictor.LoadWDLData(filepath.Join(dataDir, fmt.Sprintf("team_WDL_FT_Home_%s.csv", liga)), "home"))
		logIfUnexpectedErr("WDL Away "+liga,
			predictor.LoadWDLData(filepath.Join(dataDir, fmt.Sprintf("team_WDL_FT_Away_%s.csv", liga)), "away"))

		logIfUnexpectedErr("Corners Home "+liga,
			predictor.LoadCornersData(filepath.Join(dataDir, fmt.Sprintf("team_Corners_FT_Home_%s.csv", liga)), "home"))
		logIfUnexpectedErr("Corners Away "+liga,
			predictor.LoadCornersData(filepath.Join(dataDir, fmt.Sprintf("team_Corners_FT_Away_%s.csv", liga)), "away"))
	}
}

func main() {
	dataDir := resolveDataDir()

	if runtime.GOOS == "android" && !hasStoragePermission {
		log.Println("⚠️ Intentando abrir ajustes de almacenamiento en Android...")
		cmd := exec.Command(
			"am", "start",
			"-a", "android.settings.MANAGE_APP_ALL_FILES_ACCESS_PERMISSION",
			"-d", "package:com.futcor.predictor",
		)
		if err := cmd.Run(); err != nil {
			log.Printf("⚠️ No se pudo abrir ajustes vía am: %v", err)
		}
	}

	predictor := models.NewPredictor()

	fmt.Println("🚀 Iniciando Futbol Predictor...")
	fmt.Println("📂 Usando directorio de datos:", dataDir)

	if err := predictor.LoadTeamsFromCSV(filepath.Join(dataDir, "equipos.csv")); err != nil {
		log.Printf("⚠️ Advertencia: No se pudieron cargar los equipos: %v", err)
		log.Println("📋 Usando datos de ejemplo...")
		createSampleData(predictor)
	} else {
		fmt.Printf("✅ %d equipos cargados\n", len(predictor.Teams))
	}

	fmt.Println("📈 Cargando estadísticas de ligas...")
	loadAllStats(predictor, dataDir)

	fmt.Println("✨ Iniciando interfaz gráfica...")
	time.Sleep(300 * time.Millisecond)

	fyneUI := ui.NewFyneUI(predictor, dataDir)
	if runtime.GOOS == "android" && !hasStoragePermission {
		fyneUI.MostrarAlertaPermiso = true
	}
	fyneUI.Run()
}

func createSampleData(predictor *models.Predictor) {
	// Asegúrate de tener aquí tus equipos base si equipos.csv no existe aún
}
