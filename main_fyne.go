package main

import (
	"fmt"
	"futcor_predict/models"
	"futcor_predict/ui"
	"log"
	"time"
)

func main() {
	// Crear predictor
	predictor := models.NewPredictor()

	// Mostrar indicador de carga
	fmt.Println("🚀 Iniciando Futbol Predictor...")
	fmt.Println("📊 Cargando datos de equipos...")

	// Cargar equipos base (UNA SOLA VEZ)
	err := predictor.LoadTeamsFromCSV("data/equipos.csv")
	if err != nil {
		log.Printf("⚠️ Advertencia: No se pudieron cargar los equipos: %v", err)
		log.Println("📋 Usando datos de ejemplo...")
		createSampleData(predictor)
	} else {
		fmt.Printf("✅ %d equipos cargados\n", len(predictor.Teams))
	}

	// Cargar BTTS y WDL de todas las ligas
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
		// Intentar cargar BTTS (si los archivos existen)
		if err := predictor.LoadBTTSData(fmt.Sprintf("data/team_BTTS_1H_Overall_%s.csv", liga), "1H"); err == nil {
			fmt.Printf("  ✓ BTTS 1H %s\n", liga)
		}
		if err := predictor.LoadBTTSData(fmt.Sprintf("data/team_BTTS_2H_Overall_%s.csv", liga), "2H"); err == nil {
			fmt.Printf("  ✓ BTTS 2H %s\n", liga)
		}
		
		// Cargar xG separado para cada liga
for _, liga := range ligas {
    predictor.LoadXGSeparatedData(
        fmt.Sprintf("data/team_xG_FT_Home_%s.csv", liga),
        fmt.Sprintf("data/team_xG_FT_Away_%s.csv", liga),
    )
}

		// Intentar cargar WDL (si los archivos existen)
		if err := predictor.LoadWDLData(fmt.Sprintf("data/team_WDL_FT_Home_%s.csv", liga), "home"); err == nil {
			fmt.Printf("  ✓ WDL Home %s\n", liga)
		}
		if err := predictor.LoadWDLData(fmt.Sprintf("data/team_WDL_FT_Away_%s.csv", liga), "away"); err == nil {
			fmt.Printf("  ✓ WDL Away %s\n", liga)
		}
	}

	// Cargar datos de corners (específicos de LaLiga por ahora)
	fmt.Println("📐 Cargando estadísticas de corners...")

	if err := predictor.LoadCornersData("data/team_Corners_FT_Home_Spain LaLiga.csv", "home"); err == nil {
		fmt.Println("  ✓ Corners Home LaLiga")
	}
	if err := predictor.LoadCornersData("data/team_Corners_FT_Away_Spain LaLiga.csv", "away"); err == nil {
		fmt.Println("  ✓ Corners Away LaLiga")
	}

	fmt.Println("✨ Iniciando interfaz gráfica...")
	time.Sleep(1 * time.Second) // Pequeña pausa para leer los mensajes

	// Iniciar interfaz FYNE
	fyneUI := ui.NewFyneUI(predictor)
	fyneUI.Run()
}

// createSampleData crea datos de ejemplo si no hay CSV
func createSampleData(predictor *models.Predictor) {
	teams := []*models.Team{
		{
			ID:                "RMA",
			Name:              "Real Madrid",
			League:            "LaLiga",
			PowerRating:       95.0,
			GAx:               2.1,
			GSAx:              0.8,
			Style:             "BANDA",
			AvgCornersFor:     6.5,
			AvgCornersAgainst: 3.2,
			BTTSRate:          0.65,
			GoalsForAvg:       2.2,
			GoalsAgainstAvg:   0.9,
		},
		{
			ID:                "BAR",
			Name:              "Barcelona",
			League:            "LaLiga",
			PowerRating:       92.0,
			GAx:               2.0,
			GSAx:              0.9,
			Style:             "CENTRO",
			AvgCornersFor:     5.8,
			AvgCornersAgainst: 3.5,
			BTTSRate:          0.60,
			GoalsForAvg:       2.0,
			GoalsAgainstAvg:   1.0,
		},
		{
			ID:                "MCI",
			Name:              "Manchester City",
			League:            "Premier",
			PowerRating:       96.0,
			GAx:               2.3,
			GSAx:              0.7,
			Style:             "MIXTO",
			AvgCornersFor:     7.2,
			AvgCornersAgainst: 2.8,
			BTTSRate:          0.70,
			GoalsForAvg:       2.4,
			GoalsAgainstAvg:   0.8,
		},
	}

	for _, team := range teams {
		predictor.Teams[team.ID] = team
	}
}
