package ui

import (
	"fmt"
	"futcor_predict/models"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// Estado de la UI
type UIState int

const (
	StateMainMenu UIState = iota
	StateCornerPrediction
	StateBTTSPrediction
	StateHandicapPrediction
	StateTeamList
	StateFullAnalysis
	StateLeagueStats
	StateKellySimulator
	StateSelectingHome
	StateSelectingAway
	StateInput
)

// TerminalUI maneja la interfaz de usuario con termui
type TerminalUI struct {
	Predictor     *models.Predictor
	State         UIState
	LastState     UIState
	SelectedHome  *models.Team
	SelectedAway  *models.Team
	InputPrompt   string
	InputCallback func(string)
	InputBuffer   string
	Grid          *termui.Grid
	Components    map[string]interface{}
	HandicapValue float64
	KellyResult   [][]string
	LastResult    string
}

// NewTerminalUI crea nueva interfaz
func NewTerminalUI(predictor *models.Predictor) *TerminalUI {
	return &TerminalUI{
		Predictor:     predictor,
		State:         StateMainMenu,
		LastState:     StateMainMenu,
		Components:    make(map[string]interface{}),
		HandicapValue: -0.5,
	}
}

// Run inicia la interfaz interactiva
func (t *TerminalUI) Run() {
	if err := termui.Init(); err != nil {
		log.Fatalf("Failed to initialize termui: %v", err)
	}
	defer termui.Close()

	// Bucle principal de eventos
	t.renderCurrentState()

	uiEvents := termui.PollEvents()
	for {
		e := <-uiEvents

		if e.Type == termui.KeyboardEvent {
			if e.ID == "<C-c>" {
				return
			}
			t.handleInput(e.ID)
		}

		t.renderCurrentState()
	}
}

// renderCurrentState renderiza el estado actual
func (t *TerminalUI) renderCurrentState() {
	t.Grid = termui.NewGrid()
	termWidth, termHeight := termui.TerminalDimensions()
	t.Grid.SetRect(0, 0, termWidth, termHeight)

	switch t.State {
	case StateMainMenu:
		t.renderMainMenu()
	case StateCornerPrediction:
		t.renderCornerPrediction()
	case StateBTTSPrediction:
		t.renderBTTSPrediction()
	case StateHandicapPrediction:
		t.renderHandicapPrediction()
	case StateTeamList:
		t.renderTeamList()
	case StateFullAnalysis:
		t.renderFullAnalysis()
	case StateLeagueStats:
		t.renderLeagueStats()
	case StateKellySimulator:
		t.renderKellySimulator()
	case StateSelectingHome, StateSelectingAway:
		t.renderTeamSelection()
	case StateInput:
		t.renderInput()
	}

	termui.Render(t.Grid)
}

// handleInput maneja la entrada del teclado
func (t *TerminalUI) handleInput(key string) {
	switch t.State {
	case StateMainMenu:
		t.handleMainMenuInput(key)
	case StateCornerPrediction:
		t.handleCornerPredictionInput(key)
	case StateBTTSPrediction:
		t.handleBTTSPredictionInput(key)
	case StateHandicapPrediction:
		t.handleHandicapPredictionInput(key)
	case StateTeamList:
		t.handleTeamListInput(key)
	case StateFullAnalysis:
		t.handleFullAnalysisInput(key)
	case StateLeagueStats:
		t.handleLeagueStatsInput(key)
	case StateKellySimulator:
		t.handleKellySimulatorInput(key)
	case StateSelectingHome, StateSelectingAway:
		t.handleTeamSelectionInput(key)
	case StateInput:
		t.handleTextInput(key)
	}
}

// renderMainMenu renderiza el menú principal
func (t *TerminalUI) renderMainMenu() {
	termWidth, termHeight := termui.TerminalDimensions()

	// Título
	title := widgets.NewParagraph()
	title.Text = "⚽ FUTBOL PREDICTOR v1.0 - Modelo Avanzado para Apuestas ⚽"
	title.TextStyle.Fg = termui.ColorCyan
	title.BorderStyle.Fg = termui.ColorYellow
	title.Border = true
	title.SetRect(0, 0, termWidth, 3)

	// Menú de opciones
	var menuList *widgets.List
	if comp, exists := t.Components["menuList"]; exists {
		menuList = comp.(*widgets.List)
	} else {
		menuList = widgets.NewList()
		menuList.Title = "📊 MENÚ PRINCIPAL"
		menuList.Rows = []string{
			"[1] Predecir CORNERS (Binomial Negativa)",
			"[2] Predecir BTTS/AMBOS MARCAN (Binomial Bivariada)",
			"[3] Predecir HANDICAP Asiático",
			"[4] Listar equipos disponibles",
			"[5] Análisis completo de partido",
			"[6] Ver estadísticas liga",
			"[7] Simulador de apuestas (Kelly)",
			"[8] Salir",
		}
		menuList.TextStyle.Fg = termui.ColorWhite
		menuList.WrapText = false
		t.Components["menuList"] = menuList
	}
	menuList.SetRect(0, 3, termWidth/2, termHeight-3)

	// Panel de información
	infoPanel := widgets.NewParagraph()
	infoPanel.Title = "ℹ️ INFORMACIÓN"
	infoPanel.Text = t.getInfoText()
	infoPanel.TextStyle.Fg = termui.ColorGreen
	infoPanel.BorderStyle.Fg = termui.ColorBlue
	infoPanel.SetRect(termWidth/2, 3, termWidth, termHeight-3)

	t.Grid.Set(
		termui.NewRow(1.0,
			termui.NewCol(0.5, menuList),
			termui.NewCol(0.5, infoPanel),
		),
	)
}

// handleMainMenuInput maneja input del menú principal
func (t *TerminalUI) handleMainMenuInput(key string) {
	switch key {
	case "1":
		t.State = StateCornerPrediction
		t.LastState = StateCornerPrediction
		t.SelectedHome = nil
		t.SelectedAway = nil
	case "2":
		t.State = StateBTTSPrediction
		t.LastState = StateBTTSPrediction
		t.SelectedHome = nil
		t.SelectedAway = nil
	case "3":
		t.State = StateHandicapPrediction
		t.LastState = StateHandicapPrediction
		t.SelectedHome = nil
		t.SelectedAway = nil
	case "4":
		t.State = StateTeamList
	case "5":
		t.State = StateFullAnalysis
		t.LastState = StateFullAnalysis
		t.SelectedHome = nil
		t.SelectedAway = nil
	case "6":
		t.State = StateLeagueStats
	case "7":
		t.State = StateKellySimulator
	case "8", "q":
		termui.Close()
		return
	}
}

// renderTeamSelection renderiza la selección de equipos
func (t *TerminalUI) renderTeamSelection() {
	termWidth, termHeight := termui.TerminalDimensions()

	var list *widgets.List
	if comp, exists := t.Components["teamList"]; exists {
		list = comp.(*widgets.List)
	} else {
		list = widgets.NewList()
		list.Rows = []string{}
		for id, team := range t.Predictor.Teams {
			list.Rows = append(list.Rows, fmt.Sprintf("[%s] %s (%s)", id, team.Name, team.League))
		}
		list.TextStyle.Fg = termui.ColorWhite
		list.SelectedRowStyle.Fg = termui.ColorBlack
		list.SelectedRowStyle.Bg = termui.ColorGreen
		list.WrapText = false
		t.Components["teamList"] = list
	}

	if t.State == StateSelectingHome {
		list.Title = "🏠 Selecciona equipo LOCAL (ESC para volver)"
	} else {
		list.Title = fmt.Sprintf("✈️ Selecciona equipo VISITANTE (elegiste %s)", t.SelectedHome.Name)
	}

	list.SetRect(0, 0, termWidth, termHeight)
	t.Grid.Set(termui.NewRow(1.0, list))
}

// handleTeamSelectionInput maneja selección de equipos
func (t *TerminalUI) handleTeamSelectionInput(key string) {
	list := t.Components["teamList"].(*widgets.List)

	switch key {
	case "<Down>", "j":
		list.ScrollDown()
	case "<Up>", "k":
		list.ScrollUp()
	case "<Enter>":
		selectedIdx := list.SelectedRow
		if selectedIdx >= 0 && selectedIdx < len(list.Rows) {
			row := list.Rows[selectedIdx]
			start := strings.Index(row, "[") + 1
			end := strings.Index(row, "]")
			if start > 0 && end > start {
				teamID := row[start:end]
				team := t.Predictor.Teams[teamID]
				if team != nil {
					if t.State == StateSelectingHome {
						t.SelectedHome = team
						t.State = StateSelectingAway
					} else {
						t.SelectedAway = team
						t.State = t.LastState
					}
				}
			}
		}
	case "<Escape>":
		t.State = StateMainMenu
		t.SelectedHome = nil
		t.SelectedAway = nil
	}
}

// renderCornerPrediction renderiza predicción de corners
func (t *TerminalUI) renderCornerPrediction() {
	termWidth, termHeight := termui.TerminalDimensions()

	if t.SelectedHome == nil || t.SelectedAway == nil {
		t.LastState = StateCornerPrediction
		t.State = StateSelectingHome
		t.renderCurrentState()
		return
	}

	// Título
	title := widgets.NewParagraph()
	title.Text = "📐 PREDICCIÓN DE CORNERS - Binomial Negativa"
	title.TextStyle.Fg = termui.ColorCyan
	title.BorderStyle.Fg = termui.ColorYellow
	title.SetRect(0, 0, termWidth, 3)

	// Calcular predicción
	pred := t.Predictor.CalculateCornerExpectation(t.SelectedHome, t.SelectedAway)

	// Panel de resultados
	resultsPanel := widgets.NewParagraph()
	resultsPanel.Title = "📈 RESULTADOS"
	resultsPanel.Text = fmt.Sprintf(
		"🏠 %s: %.2f corners\n"+
			"✈️ %s: %.2f corners\n"+
			"📊 Total esperado: %.2f\n\n"+
			"📉 Varianza: %.2f\n"+
			"🎯 Rango 68%%: [%.0f-%.0f]\n\n"+
			"[ESC] Volver al menú",
		t.SelectedHome.Name, pred.HomeExpected,
		t.SelectedAway.Name, pred.AwayExpected,
		pred.TotalExpected,
		pred.TotalExpected*0.3,
		pred.TotalExpected-pred.TotalExpected*0.3,
		pred.TotalExpected+pred.TotalExpected*0.3,
	)
	resultsPanel.TextStyle.Fg = termui.ColorGreen
	resultsPanel.BorderStyle.Fg = termui.ColorBlue
	resultsPanel.SetRect(0, 3, termWidth/2, termHeight-3)

	// Gráfico de distribución
	barChart := widgets.NewBarChart()
	barChart.Title = "Distribución de Corners"
	barChart.BarColors = []termui.Color{termui.ColorGreen, termui.ColorYellow}
	barChart.LabelStyles = []termui.Style{termui.NewStyle(termui.ColorWhite)}
	barChart.NumStyles = []termui.Style{termui.NewStyle(termui.ColorBlack)}
	barChart.SetRect(termWidth/2, 3, termWidth, termHeight-3)

	var totals []float64
	var labels []string
	for total := 8; total <= 15; total++ {
		if prob, exists := pred.Distribution[total]; exists && prob > 0.01 {
			totals = append(totals, prob*100)
			labels = append(labels, fmt.Sprintf("%d", total))
		}
	}
	barChart.Data = totals
	barChart.Labels = labels

	t.Grid.Set(
		termui.NewRow(1.0/8, title),
		termui.NewRow(7.0/8,
			termui.NewCol(0.5, resultsPanel),
			termui.NewCol(0.5, barChart),
		),
	)
}

// handleCornerPredictionInput maneja input en corners
func (t *TerminalUI) handleCornerPredictionInput(key string) {
	if key == "<Escape>" {
		t.State = StateMainMenu
		t.SelectedHome = nil
		t.SelectedAway = nil
	}
}

// renderBTTSPrediction renderiza predicción BTTS
func (t *TerminalUI) renderBTTSPrediction() {
	termWidth, termHeight := termui.TerminalDimensions()

	if t.SelectedHome == nil || t.SelectedAway == nil {
		t.LastState = StateBTTSPrediction
		t.State = StateSelectingHome
		t.renderCurrentState()
		return
	}

	title := widgets.NewParagraph()
	title.Text = "⚽ PREDICCIÓN BTTS - Binomial Bivariada"
	title.TextStyle.Fg = termui.ColorCyan
	title.BorderStyle.Fg = termui.ColorYellow
	title.SetRect(0, 0, termWidth, 3)

	// Calcular predicción
	pred := t.Predictor.CalculateBTTSProbability(t.SelectedHome, t.SelectedAway)

	// --- Mostrar datos reales de los equipos ---
	// Usamos los campos BTTSRate (global) y, si existen, BTTS1H y BTTS2H
	homeBTTS := t.SelectedHome.BTTSRate * 100
	awayBTTS := t.SelectedAway.BTTSRate * 100

	// Preparamos texto adicional para mitades si están disponibles
	halfInfo := ""
	if t.SelectedHome.BTTS1H > 0 || t.SelectedHome.BTTS2H > 0 {
		halfInfo = fmt.Sprintf(
			"  • %s: 1H %.1f%% | 2H %.1f%%\n",
			t.SelectedHome.Name,
			t.SelectedHome.BTTS1H*100,
			t.SelectedHome.BTTS2H*100,
		)
	}
	if t.SelectedAway.BTTS1H > 0 || t.SelectedAway.BTTS2H > 0 {
		halfInfo += fmt.Sprintf(
			"  • %s: 1H %.1f%% | 2H %.1f%%\n",
			t.SelectedAway.Name,
			t.SelectedAway.BTTS1H*100,
			t.SelectedAway.BTTS2H*100,
		)
	}

	// Panel de resultados
	resultsPanel := widgets.NewParagraph()
	resultsPanel.Title = "📊 RESULTADOS"

	resultText := fmt.Sprintf(
		"🏠 %s vs ✈️ %s\n\n"+
			"📈 Probabilidad BTTS: %.1f%%\n"+
			"💰 Cuota justa SÍ: %.2f\n"+
			"💰 Cuota justa NO: %.2f\n\n"+
			"📊 Datos reales BTTS (global):\n"+
			"  • %s: %.1f%%\n"+
			"  • %s: %.1f%%\n",
		t.SelectedHome.Name, t.SelectedAway.Name,
		pred.Probability*100,
		pred.YesOdds,
		1/(1-pred.Probability),
		t.SelectedHome.Name, homeBTTS,
		t.SelectedAway.Name, awayBTTS,
	)

	// Añadir información de mitades si existe
	if halfInfo != "" {
		resultText += "\n📊 Por mitades:\n" + halfInfo
	}

	if t.LastResult != "" {
		resultText += "\n" + t.LastResult + "\n"
	}

	resultText += "\n[I] Ingresar cuota real\n[ESC] Volver"

	resultsPanel.Text = resultText
	resultsPanel.TextStyle.Fg = termui.ColorGreen
	resultsPanel.BorderStyle.Fg = termui.ColorBlue
	resultsPanel.SetRect(0, 3, termWidth/2, termHeight-3)

	// Gauge
	gauge := widgets.NewGauge()
	gauge.Title = "Probabilidad BTTS"
	gauge.Percent = int(pred.Probability * 100)
	gauge.BarColor = termui.ColorGreen
	gauge.BorderStyle.Fg = termui.ColorWhite
	gauge.TitleStyle.Fg = termui.ColorCyan
	gauge.SetRect(termWidth/2, 3, termWidth, 8)

	t.Grid.Set(
		termui.NewRow(1.0/8, title),
		termui.NewRow(7.0/8,
			termui.NewCol(0.5, resultsPanel),
			termui.NewCol(0.5, gauge),
		),
	)
}

// handleBTTSPredictionInput maneja input en BTTS
func (t *TerminalUI) handleBTTSPredictionInput(key string) {
	switch key {
	case "i", "I":
		t.State = StateInput
		t.InputPrompt = "Ingresa cuota para BTTS SÍ: "
		t.InputCallback = func(input string) {
			odds, err := strconv.ParseFloat(input, 64)
			if err == nil && odds > 1 {
				pred := t.Predictor.CalculateBTTSProbability(t.SelectedHome, t.SelectedAway)
				recommend, kelly, edge := t.Predictor.RecommendBet(pred.Probability, odds, 0.05)

				if recommend {
					t.LastResult = fmt.Sprintf("✅ APOSTAR - Kelly: %.2f%% | Edge: %.2f%%", kelly*100, edge)
				} else {
					t.LastResult = fmt.Sprintf("❌ NO APOSTAR - Kelly: %.2f%% | Edge: %.2f%%", kelly*100, edge)
				}
			}
			t.State = StateBTTSPrediction
		}
	case "<Escape>":
		t.State = StateMainMenu
		t.SelectedHome = nil
		t.SelectedAway = nil
		t.LastResult = ""
	}
}

// renderHandicapPrediction renderiza predicción de handicap
func (t *TerminalUI) renderHandicapPrediction() {
	termWidth, termHeight := termui.TerminalDimensions()

	if t.SelectedHome == nil || t.SelectedAway == nil {
		t.LastState = StateHandicapPrediction
		t.State = StateSelectingHome
		t.renderCurrentState()
		return
	}

	title := widgets.NewParagraph()
	title.Text = "🎯 PREDICCIÓN HANDICAP ASIÁTICO"
	title.TextStyle.Fg = termui.ColorCyan
	title.BorderStyle.Fg = termui.ColorYellow
	title.SetRect(0, 0, termWidth, 3)

	pred := t.Predictor.CalculateHandicap(t.SelectedHome, t.SelectedAway, t.HandicapValue)

	// Panel de resultados
	resultsPanel := widgets.NewTable()
	resultsPanel.Title = "📊 PROBABILIDADES"
	resultsPanel.Rows = [][]string{
		{"Mercado", "Probabilidad", "Cuota Justa"},
		{
			fmt.Sprintf("Local %.1f", t.HandicapValue),
			fmt.Sprintf("%.1f%%", pred.HomeProb*100),
			fmt.Sprintf("%.2f", 1/pred.HomeProb),
		},
		{
			fmt.Sprintf("Visitante %+.1f", -t.HandicapValue),
			fmt.Sprintf("%.1f%%", pred.AwayProb*100),
			fmt.Sprintf("%.2f", 1/pred.AwayProb),
		},
	}
	resultsPanel.TextStyle = termui.NewStyle(termui.ColorWhite)
	resultsPanel.RowSeparator = true
	resultsPanel.SetRect(0, 3, termWidth, termHeight-6)

	// Instrucciones
	info := widgets.NewParagraph()
	info.Text = fmt.Sprintf(
		"Diferencia esperada: %.2f goles\n"+
			"Handicap actual: %.2f\n"+
			"[H] Cambiar handicap\n"+
			"[ESC] Volver al menú",
		pred.ExpectedValue,
		t.HandicapValue,
	)
	info.TextStyle.Fg = termui.ColorYellow
	info.Border = false
	info.SetRect(0, termHeight-5, termWidth, termHeight-3)

	t.Grid.Set(
		termui.NewRow(1.0/8, title),
		termui.NewRow(6.0/8, resultsPanel),
		termui.NewRow(1.0/8, info),
	)
}

// handleHandicapPredictionInput maneja input en handicap
func (t *TerminalUI) handleHandicapPredictionInput(key string) {
	switch key {
	case "h", "H":
		t.State = StateInput
		t.InputPrompt = "Ingresa handicap (ej. -0.5, +0.75): "
		t.InputCallback = func(input string) {
			if h, err := strconv.ParseFloat(input, 64); err == nil {
				t.HandicapValue = h
			}
			t.State = StateHandicapPrediction
		}
	case "<Escape>":
		t.State = StateMainMenu
		t.SelectedHome = nil
		t.SelectedAway = nil
		t.HandicapValue = -0.5
	}
}

// renderTeamList renderiza lista de equipos
func (t *TerminalUI) renderTeamList() {
	termWidth, termHeight := termui.TerminalDimensions()

	title := widgets.NewParagraph()
	title.Text = "📋 EQUIPOS DISPONIBLES"
	title.TextStyle.Fg = termui.ColorCyan
	title.BorderStyle.Fg = termui.ColorYellow
	title.SetRect(0, 0, termWidth, 3)

	// Tabla de equipos
	teamTable := widgets.NewTable()
	teamTable.Title = "Estadísticas por equipo"
	teamTable.Rows = [][]string{
		{"ID", "Nombre", "Liga", "Power", "Estilo", "Corners", "BTTS%"},
	}

	for id, team := range t.Predictor.Teams {
		teamTable.Rows = append(teamTable.Rows, []string{
			id,
			team.Name,
			team.League,
			fmt.Sprintf("%.1f", team.PowerRating),
			team.Style,
			fmt.Sprintf("%.1f", team.AvgCornersFor),
			fmt.Sprintf("%.0f%%", team.BTTSRate*100),
		})
	}

	teamTable.TextStyle = termui.NewStyle(termui.ColorWhite)
	teamTable.RowSeparator = true
	teamTable.SetRect(0, 3, termWidth, termHeight-3)

	t.Grid.Set(
		termui.NewRow(1.0/8, title),
		termui.NewRow(7.0/8, teamTable),
	)
}

// handleTeamListInput maneja input en lista de equipos
func (t *TerminalUI) handleTeamListInput(key string) {
	if key == "<Escape>" || key == "q" {
		t.State = StateMainMenu
	}
}

// renderFullAnalysis renderiza análisis completo
func (t *TerminalUI) renderFullAnalysis() {
	termWidth, termHeight := termui.TerminalDimensions()

	if t.SelectedHome == nil || t.SelectedAway == nil {
		t.LastState = StateFullAnalysis
		t.State = StateSelectingHome
		t.renderCurrentState()
		return
	}

	title := widgets.NewParagraph()
	title.Text = "🔍 ANÁLISIS COMPLETO DE PARTIDO"
	title.TextStyle.Fg = termui.ColorCyan
	title.BorderStyle.Fg = termui.ColorYellow
	title.SetRect(0, 0, termWidth, 3)

	// Calcular predicciones
	corners := t.Predictor.CalculateCornerExpectation(t.SelectedHome, t.SelectedAway)
	btts := t.Predictor.CalculateBTTSProbability(t.SelectedHome, t.SelectedAway)
	handicap := t.Predictor.CalculateHandicap(t.SelectedHome, t.SelectedAway, -0.5)

	// Panel de métricas
	metricsPanel := widgets.NewTable()
	metricsPanel.Title = "📊 MÉTRICAS PREDICHAS"
	metricsPanel.Rows = [][]string{
		{"Métrica", t.SelectedHome.Name, t.SelectedAway.Name},
		{"Power Rating", fmt.Sprintf("%.1f", t.SelectedHome.PowerRating), fmt.Sprintf("%.1f", t.SelectedAway.PowerRating)},
		{"Corners Esp.", fmt.Sprintf("%.1f", corners.HomeExpected), fmt.Sprintf("%.1f", corners.AwayExpected)},
		{"BTTS Prob", fmt.Sprintf("%.1f%%", btts.Probability*100), "-"},
		{"Victoria", fmt.Sprintf("%.1f%%", handicap.HomeProb*100), fmt.Sprintf("%.1f%%", handicap.AwayProb*100)},
		{"GAx/GSAx", fmt.Sprintf("%.1f/%.1f", t.SelectedHome.GAx, t.SelectedHome.GSAx), fmt.Sprintf("%.1f/%.1f", t.SelectedAway.GAx, t.SelectedAway.GSAx)},
	}
	metricsPanel.TextStyle = termui.NewStyle(termui.ColorWhite)
	metricsPanel.RowSeparator = true
	metricsPanel.SetRect(0, 3, termWidth, termHeight-3)

	t.Grid.Set(
		termui.NewRow(1.0/8, title),
		termui.NewRow(7.0/8, metricsPanel),
	)
}

// handleFullAnalysisInput maneja input en análisis completo
func (t *TerminalUI) handleFullAnalysisInput(key string) {
	if key == "<Escape>" {
		t.State = StateMainMenu
		t.SelectedHome = nil
		t.SelectedAway = nil
	}
}

// renderLeagueStats renderiza estadísticas de liga
func (t *TerminalUI) renderLeagueStats() {
	termWidth, termHeight := termui.TerminalDimensions()

	title := widgets.NewParagraph()
	title.Text = "📊 ESTADÍSTICAS POR LIGA"
	title.TextStyle.Fg = termui.ColorCyan
	title.BorderStyle.Fg = termui.ColorYellow
	title.SetRect(0, 0, termWidth, 3)

	// Calcular estadísticas por liga
	leagueStats := make(map[string]map[string]float64)
	for _, team := range t.Predictor.Teams {
		if _, exists := leagueStats[team.League]; !exists {
			leagueStats[team.League] = map[string]float64{
				"power": 0, "corners": 0, "btts": 0, "count": 0,
			}
		}
		stats := leagueStats[team.League]
		stats["power"] += team.PowerRating
		stats["corners"] += team.AvgCornersFor
		stats["btts"] += team.BTTSRate
		stats["count"]++
	}

	// Tabla de estadísticas
	statsTable := widgets.NewTable()
	statsTable.Title = "Promedios por liga"
	statsTable.Rows = [][]string{
		{"Liga", "Power", "Corners", "BTTS", "Equipos"},
	}

	for league, stats := range leagueStats {
		count := stats["count"]
		statsTable.Rows = append(statsTable.Rows, []string{
			league,
			fmt.Sprintf("%.1f", stats["power"]/count),
			fmt.Sprintf("%.1f", stats["corners"]/count),
			fmt.Sprintf("%.0f%%", (stats["btts"]/count)*100),
			fmt.Sprintf("%.0f", count),
		})
	}

	statsTable.TextStyle = termui.NewStyle(termui.ColorWhite)
	statsTable.RowSeparator = true
	statsTable.SetRect(0, 3, termWidth, termHeight-3)

	t.Grid.Set(
		termui.NewRow(1.0/8, title),
		termui.NewRow(7.0/8, statsTable),
	)
}

// handleLeagueStatsInput maneja input en estadísticas de liga
func (t *TerminalUI) handleLeagueStatsInput(key string) {
	if key == "<Escape>" || key == "q" {
		t.State = StateMainMenu
	}
}

// renderKellySimulator renderiza simulador de Kelly
func (t *TerminalUI) renderKellySimulator() {
	termWidth, termHeight := termui.TerminalDimensions()

	title := widgets.NewParagraph()
	title.Text = "💰 SIMULADOR CRITERIO DE KELLY"
	title.TextStyle.Fg = termui.ColorCyan
	title.BorderStyle.Fg = termui.ColorYellow
	title.SetRect(0, 0, termWidth, 3)

	// Instrucciones
	info := widgets.NewParagraph()
	info.Text = "Presiona [P] para ingresar datos\n" +
		"[ESC] Volver al menú"
	info.TextStyle.Fg = termui.ColorYellow
	info.Border = false
	info.SetRect(0, 3, termWidth, 6)

	// Resultados
	resultsPanel := widgets.NewTable()
	resultsPanel.Title = "📊 RESULTADOS KELLY"

	if t.KellyResult != nil {
		resultsPanel.Rows = t.KellyResult
	} else {
		resultsPanel.Rows = [][]string{
			{"Métrica", "Valor"},
			{"Kelly %", "-"},
			{"Edge %", "-"},
			{"Apuesta $", "-"},
			{"Recomendación", "-"},
		}
	}

	resultsPanel.TextStyle = termui.NewStyle(termui.ColorWhite)
	resultsPanel.SetRect(0, 7, termWidth, termHeight-3)

	t.Grid.Set(
		termui.NewRow(1.0/8, title),
		termui.NewRow(1.0/8, info),
		termui.NewRow(6.0/8, resultsPanel),
	)
}

// handleKellySimulatorInput maneja input en simulador de Kelly
func (t *TerminalUI) handleKellySimulatorInput(key string) {
	switch key {
	case "p", "P":
		t.State = StateInput
		t.InputPrompt = "Ingresa probabilidad real (%): "
		t.InputCallback = t.handleKellyProbInput
	case "<Escape>":
		t.State = StateMainMenu
	}
}

// handleKellyProbInput maneja input de probabilidad
func (t *TerminalUI) handleKellyProbInput(probStr string) {
	prob, err := strconv.ParseFloat(probStr, 64)
	if err != nil {
		t.State = StateKellySimulator
		return
	}
	prob = prob / 100
	t.Components["kellyProb"] = prob

	t.State = StateInput
	t.InputPrompt = "Ingresa cuota decimal: "
	t.InputCallback = t.handleKellyOddsInput
}

// handleKellyOddsInput maneja input de cuota
func (t *TerminalUI) handleKellyOddsInput(oddsStr string) {
	odds, err := strconv.ParseFloat(oddsStr, 64)
	if err != nil {
		t.State = StateKellySimulator
		return
	}
	t.Components["kellyOdds"] = odds

	t.State = StateInput
	t.InputPrompt = "Ingresa bankroll ($): "
	t.InputCallback = t.handleKellyBankInput
}

// handleKellyBankInput maneja input de bankroll
func (t *TerminalUI) handleKellyBankInput(bankStr string) {
	bank, err := strconv.ParseFloat(bankStr, 64)
	if err != nil {
		t.State = StateKellySimulator
		return
	}

	prob := t.Components["kellyProb"].(float64)
	odds := t.Components["kellyOdds"].(float64)

	kelly := t.Predictor.KellyCriterion(prob, odds)
	edge := t.Predictor.Edge(prob, odds)
	betAmount := bank * kelly

	recommendation := "❌ NO APOSTAR"
	if kelly > 0.05 {
		recommendation = "✅ APOSTAR (valor alto)"
	} else if kelly > 0 {
		recommendation = "⚠️ APOSTAR (valor bajo)"
	}

	t.KellyResult = [][]string{
		{"Métrica", "Valor"},
		{"Kelly %", fmt.Sprintf("%.2f%%", kelly*100)},
		{"Edge %", fmt.Sprintf("%.2f%%", edge)},
		{"Apuesta $", fmt.Sprintf("$%.2f", betAmount)},
		{"Recomendación", recommendation},
	}

	t.State = StateKellySimulator
}

// renderInput renderiza entrada de texto
func (t *TerminalUI) renderInput() {
	termWidth, termHeight := termui.TerminalDimensions()

	input := widgets.NewParagraph()
	input.Title = t.InputPrompt
	input.Text = t.InputBuffer
	input.BorderStyle.Fg = termui.ColorYellow
	input.SetRect(termWidth/3, termHeight/2-3, 2*termWidth/3, termHeight/2+3)

	t.Grid.Set(termui.NewRow(1.0, input))
}

// handleTextInput maneja entrada de texto
func (t *TerminalUI) handleTextInput(key string) {
	switch key {
	case "<Enter>":
		if t.InputBuffer != "" && t.InputCallback != nil {
			t.InputCallback(t.InputBuffer)
			t.InputBuffer = ""
		}
	case "<Escape>":
		t.State = StateMainMenu
		t.InputBuffer = ""
	case "<Backspace>":
		if len(t.InputBuffer) > 0 {
			t.InputBuffer = t.InputBuffer[:len(t.InputBuffer)-1]
		}
	default:
		if len(key) == 1 && key >= " " && key <= "~" {
			t.InputBuffer += key
		}
	}
}

// getInfoText retorna texto de información
func (t *TerminalUI) getInfoText() string {
	return fmt.Sprintf(
		"📊 MODELOS ACTIVOS:\n"+
			"• Corners: Binomial Negativa\n"+
			"• BTTS: Binomial Bivariada\n"+
			"• Handicap: Distribución Normal\n\n"+
			"📈 MÉTRICAS:\n"+
			"• Power Rating\n"+
			"• GAx / GSAx\n"+
			"• Estilo de juego\n\n"+
			"💰 GESTIÓN:\n"+
			"• Criterio de Kelly\n"+
			"• Edge Calculator\n\n"+
			"📅 EQUIPOS: %d\n"+
			"🔄 Última actualización: %s\n\n"+
			"⌨️ Navegación:\n"+
			"• Números: Seleccionar opción\n"+
			"• ↑/↓: Navegar listas\n"+
			"• Enter: Confirmar\n"+
			"• ESC: Volver",
		len(t.Predictor.Teams),
		time.Now().Format("02/01/2006 15:04"),
	)
}
