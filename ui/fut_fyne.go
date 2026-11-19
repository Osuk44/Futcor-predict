package ui

import (
	"context"
	"fmt"
	"futcor_predict/models"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// predictionCache almacena predicciones para evitar recálculos
type predictionCache struct {
	Home      *models.Team
	Away      *models.Team
	Corners   *models.CornerPrediction
	BTTS      *models.BTTSPrediction
	Handicap  *models.HandicapPrediction
	Timestamp time.Time
}

// TeamSelector componente personalizado para selección de equipos
type TeamSelector struct {
	entry      *widget.Entry
	select_    *widget.Select
	container  *fyne.Container
	onSelected func(team *models.Team)
}

// FyneUI estructura principal de la interfaz gráfica
type FyneUI struct {
	Predictor *models.Predictor
	Window    fyne.Window
	App       fyne.App

	// Componentes UI personalizados
	homeSelector   *TeamSelector
	awaySelector   *TeamSelector
	resultText     *widget.Label
	bttsResult     *widget.Label
	handicapResult *widget.Label
	kellyResult    *widget.Label
	oddsEntry      *widget.Entry
	probBinding    binding.Float
	handicapEntry  *widget.Entry

	// Estado protegido con mutex
	mu          sync.RWMutex
	currentHome *models.Team
	currentAway *models.Team

	// Control de concurrencia
	calcCtx    context.Context
	calcCancel context.CancelFunc
	calcMu     sync.Mutex

	// Caché
	cache   *predictionCache
	cacheMu sync.RWMutex

	// Gráficos
	cornersChart *widget.Label
}

// NewFyneUI crea una nueva interfaz gráfica
func NewFyneUI(predictor *models.Predictor) *FyneUI {
	myApp := app.NewWithID("com.futcor.predictor")

	// ← APLICAR TEMA PERSONALIZADO (automático, afecta toda la app)
	myApp.Settings().SetTheme(&DarkSoccerTheme{})

	myWindow := myApp.NewWindow("⚽ Futbol Predictor - Modelo Avanzado")
	myWindow.Resize(fyne.NewSize(900, 700))
	myWindow.CenterOnScreen()

	return &FyneUI{
		Predictor:   predictor,
		App:         myApp,
		Window:      myWindow,
		probBinding: binding.NewFloat(),
	}
}

// Run inicia la interfaz gráfica
func (f *FyneUI) Run() {
	f.probBinding.Set(0.5)
	f.Window.SetContent(f.buildMainUI())
	f.Window.ShowAndRun()
}

// Métodos thread-safe para acceder al estado
func (f *FyneUI) getTeams() (*models.Team, *models.Team) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.currentHome, f.currentAway
}

func (f *FyneUI) setTeams(home, away *models.Team) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.currentHome = home
	f.currentAway = away
}

// cancelCurrentCalculation cancela cualquier cálculo en progreso
func (f *FyneUI) cancelCurrentCalculation() {
	f.calcMu.Lock()
	defer f.calcMu.Unlock()
	if f.calcCancel != nil {
		f.calcCancel()
	}
	f.calcCtx, f.calcCancel = context.WithCancel(context.Background())
}

// createTeamSelector crea un selector de equipo híbrido (Entry + Select)
func (f *FyneUI) createTeamSelector(placeholder string, isHome bool) *TeamSelector {
	ts := &TeamSelector{}

	// Entry para escribir
	ts.entry = widget.NewEntry()
	ts.entry.SetPlaceHolder(placeholder)

	// Select que aparece solo cuando hay coincidencias
	ts.select_ = widget.NewSelect([]string{}, func(selected string) {
		if selected == "" {
			return
		}
		// Buscar el equipo seleccionado
		for _, team := range f.Predictor.Teams {
			if team.Name == selected {
				ts.entry.SetText(selected)
				if isHome {
					f.setTeams(team, f.currentAway)
				} else {
					f.setTeams(f.currentHome, team)
				}
				ts.onSelected(team)
				break
			}
		}
		// Ocultar el select después de seleccionar
		ts.select_.ClearSelected()
		ts.select_.Options = []string{}
		ts.select_.Refresh()
		ts.select_.Hide()
	})
	ts.select_.PlaceHolder = "Haz clic para ver coincidencias..."
	ts.select_.Hide() // Inicialmente oculto

	// Layout: Entry arriba, Select abajo (solo visible cuando hay opciones)
	ts.container = container.NewVBox(ts.entry, ts.select_)

	// Lógica de búsqueda con debounce simple
	var lastSearch string
	ts.entry.OnChanged = func(text string) {
		// Ocultar select si el texto es muy corto
		if len(text) < 2 {
			ts.select_.Hide()
			ts.select_.Options = []string{}
			return
		}

		// Evitar búsquedas repetidas del mismo texto
		if text == lastSearch {
			return
		}
		lastSearch = text

		// Buscar coincidencias
		var matches []string
		textLower := strings.ToLower(text)
		for _, team := range f.Predictor.Teams {
			if strings.Contains(strings.ToLower(team.Name), textLower) {
				matches = append(matches, team.Name)
			}
		}

		// Mostrar select solo si hay coincidencias
		if len(matches) > 0 {
			ts.select_.Options = matches
			ts.select_.Refresh()
			ts.select_.Show()
		} else {
			ts.select_.Hide()
		}
	}

	return ts
}

// buildMainUI construye la interfaz principal
func (f *FyneUI) buildMainUI() fyne.CanvasObject {
	// Título
	title := canvas.NewText("⚽ FUTBOL PREDICTOR v2.0", theme.PrimaryColor())
	title.TextSize = 28
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	subtitle := widget.NewLabelWithStyle(
		"Modelo predictivo avanzado con datos en tiempo real",
		fyne.TextAlignCenter,
		fyne.TextStyle{Italic: true},
	)

	// Selectores de equipos personalizados
	f.homeSelector = f.createTeamSelector("🏠 Escribe equipo LOCAL...", true)
	f.homeSelector.onSelected = func(t *models.Team) { f.onTeamSelected() }

	f.awaySelector = f.createTeamSelector("✈️ Escribe equipo VISITANTE...", false)
	f.awaySelector.onSelected = func(t *models.Team) { f.onTeamSelected() }

	// Botón de búsqueda
	searchBtn := widget.NewButtonWithIcon("Buscar / Actualizar", theme.SearchIcon(), func() {
		f.handleSearch()
	})

	// Tabs para diferentes predicciones
	tabs := container.NewAppTabs(
		container.NewTabItem("📐 CORNERS", f.buildCornersTab()),
		container.NewTabItem("⚽ BTTS", f.buildBTTSTab()),
		container.NewTabItem("🎯 HANDICAP", f.buildHandicapTab()),
		container.NewTabItem("💰 KELLY", f.buildKellyTab()),
		container.NewTabItem("🏆 1X2", f.buildOutcomeTab()),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	// Layout principal
	content := container.NewBorder(
		container.NewVBox(
			title,
			subtitle,
			widget.NewSeparator(),
			container.NewGridWithColumns(2,
				container.NewVBox(
					widget.NewLabelWithStyle("LOCAL", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
					f.homeSelector.container,
				),
				container.NewVBox(
					widget.NewLabelWithStyle("VISITANTE", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
					f.awaySelector.container,
				),
			),
			searchBtn,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		tabs,
	)

	return content
}

// onTeamSelected se llama cuando se selecciona un equipo
func (f *FyneUI) onTeamSelected() {
	home, away := f.getTeams()
	if home != nil && away != nil {
		// Validar que no sean el mismo equipo
		if home.Name == away.Name {
			f.showError("El equipo local y visitante deben ser diferentes")
			return
		}
		f.updateCornersAsync()
		f.updateBTTSAsync()
	}
}

// handleSearch fuerza la búsqueda manual
func (f *FyneUI) handleSearch() {
	homeText := f.homeSelector.entry.Text
	awayText := f.awaySelector.entry.Text

	if homeText == "" || awayText == "" {
		f.showError("Debes escribir ambos equipos")
		return
	}

	var home, away *models.Team

	// Búsqueda exacta primero
	for _, team := range f.Predictor.Teams {
		if strings.EqualFold(team.Name, homeText) {
			home = team
			f.homeSelector.entry.SetText(team.Name) // Normalizar nombre
		}
		if strings.EqualFold(team.Name, awayText) {
			away = team
			f.awaySelector.entry.SetText(team.Name)
		}
	}

	// Si no hay exacta, buscar parcial
	if home == nil {
		for _, team := range f.Predictor.Teams {
			if strings.Contains(strings.ToLower(team.Name), strings.ToLower(homeText)) {
				home = team
				f.homeSelector.entry.SetText(team.Name)
				break
			}
		}
	}
	if away == nil {
		for _, team := range f.Predictor.Teams {
			if strings.Contains(strings.ToLower(team.Name), strings.ToLower(awayText)) {
				away = team
				f.awaySelector.entry.SetText(team.Name)
				break
			}
		}
	}

	if home == nil {
		f.showError(fmt.Sprintf("Equipo no encontrado: %s", homeText))
		return
	}
	if away == nil {
		f.showError(fmt.Sprintf("Equipo no encontrado: %s", awayText))
		return
	}

	f.setTeams(home, away)
	f.onTeamSelected()
}

// buildCornersTab construye la pestaña de corners
func (f *FyneUI) buildCornersTab() fyne.CanvasObject {
	f.resultText = widget.NewLabel("Selecciona dos equipos para ver la predicción")
	f.resultText.Wrapping = fyne.TextWrapWord
	f.resultText.Alignment = fyne.TextAlignLeading

	f.cornersChart = widget.NewLabel("📊 Distribución de corners aparecerá aquí")
	f.cornersChart.Alignment = fyne.TextAlignCenter
	f.cornersChart.Wrapping = fyne.TextWrapWord

	calcBtn := widget.NewButtonWithIcon("Calcular Predicción", theme.ViewRefreshIcon(), func() {
		f.updateCornersAsync()
	})

	split := container.NewVSplit(
		container.NewVScroll(f.resultText),
		container.NewVScroll(f.cornersChart),
	)
	split.Offset = 0.6

	content := container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), calcBtn),
		nil, nil,
		split,
	)

	return content
}

// buildBTTSTab construye la pestaña de BTTS
func (f *FyneUI) buildBTTSTab() fyne.CanvasObject {
	f.bttsResult = widget.NewLabel("Selecciona equipos para predecir BTTS")
	f.bttsResult.Wrapping = fyne.TextWrapWord
	f.bttsResult.Alignment = fyne.TextAlignLeading

	calcBtn := widget.NewButtonWithIcon("Calcular BTTS", theme.ViewRefreshIcon(), func() {
		f.updateBTTSAsync()
	})

	return container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), calcBtn),
		nil, nil,
		container.NewVScroll(f.bttsResult),
	)
}

// buildHandicapTab construye la pestaña de handicap
func (f *FyneUI) buildHandicapTab() fyne.CanvasObject {
	f.handicapResult = widget.NewLabel("Selecciona equipos para predecir Handicap")
	f.handicapResult.Wrapping = fyne.TextWrapWord
	f.handicapResult.Alignment = fyne.TextAlignLeading

	f.handicapEntry = widget.NewEntry()
	f.handicapEntry.SetPlaceHolder("Ej: -0.5, +0.75, 0.0")
	f.handicapEntry.SetText("-0.5")
	f.handicapEntry.Validator = func(s string) error {
		if s == "" {
			return nil
		}
		_, err := strconv.ParseFloat(s, 64)
		return err
	}

	calcBtn := widget.NewButtonWithIcon("Calcular Handicap", theme.ViewRefreshIcon(), func() {
		handicap, err := strconv.ParseFloat(f.handicapEntry.Text, 64)
		if err != nil {
			f.showError("Handicap inválido")
			return
		}
		f.updateHandicapAsync(handicap)
	})

	return container.NewBorder(
		container.NewVBox(
			widget.NewLabel("Handicap Asiático:"),
			f.handicapEntry,
		),
		container.NewVBox(widget.NewSeparator(), calcBtn),
		nil, nil,
		container.NewVScroll(f.handicapResult),
	)
}

// buildKellyTab construye la pestaña del simulador Kelly
func (f *FyneUI) buildKellyTab() fyne.CanvasObject {
	f.oddsEntry = widget.NewEntry()
	f.oddsEntry.SetPlaceHolder("Ej: 2.10")
	f.oddsEntry.Validator = func(s string) error {
		if s == "" {
			return nil
		}
		val, err := strconv.ParseFloat(s, 64)
		if err != nil || val <= 1.0 {
			return fmt.Errorf("cuota debe ser > 1.0")
		}
		return nil
	}

	probLabel := widget.NewLabelWithData(
		binding.FloatToStringWithFormat(f.probBinding, "Probabilidad: %.0f%%"),
	)

	slider := widget.NewSliderWithData(0, 100, f.probBinding)
	slider.Step = 1

	f.kellyResult = widget.NewLabel("")
	f.kellyResult.Wrapping = fyne.TextWrapWord

	calcBtn := widget.NewButtonWithIcon("Calcular Kelly", theme.ConfirmIcon(), func() {
		f.calculateKelly()
	})

	return container.NewVBox(
		widget.NewLabelWithStyle("CRITERIO DE KELLY", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Probabilidad modelo:"),
			probLabel,
		),
		slider,
		container.NewGridWithColumns(2,
			widget.NewLabel("Cuota ofrecida:"),
			f.oddsEntry,
		),
		calcBtn,
		widget.NewSeparator(),
		f.kellyResult,
	)
}

func (f *FyneUI) buildOutcomeTab() fyne.CanvasObject {
	// Crear un label para mostrar resultados
	outcomeResult := widget.NewLabel("Selecciona equipos para predecir el resultado")
	outcomeResult.Wrapping = fyne.TextWrapWord

	calcBtn := widget.NewButtonWithIcon("Calcular Resultado", theme.ViewRefreshIcon(), func() {
		f.updateOutcomeAsync(outcomeResult)
	})

	return container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), calcBtn),
		nil, nil,
		container.NewVScroll(outcomeResult),
	)
}
// updateCornersAsync calcula corners en segundo plano con cancelación
func (f *FyneUI) updateCornersAsync() {
	home, away := f.getTeams()
	if home == nil || away == nil {
		f.resultText.SetText("❌ Selecciona ambos equipos primero")
		return
	}

	// Verificar caché
	f.cacheMu.RLock()
	if f.cache != nil && f.cache.Home == home && f.cache.Away == away &&
		time.Since(f.cache.Timestamp) < 5*time.Minute {
		f.displayCornersResult(f.cache.Corners, home, away)
		f.cacheMu.RUnlock()
		return
	}
	f.cacheMu.RUnlock()

	f.cancelCurrentCalculation()
	ctx := f.calcCtx

	f.resultText.SetText("🔄 Calculando predicción de corners...")
	f.cornersChart.SetText("🔄 Cargando distribución...")

	go func(ctx context.Context, home, away *models.Team) {
		defer func() {
			if r := recover(); r != nil {
				fyne.Do(func() {
					f.showError(fmt.Sprintf("Error en cálculo: %v", r))
				})
			}
		}()

		select {
		case <-ctx.Done():
			return
		default:
		}

		pred := f.Predictor.CalculateCornerExpectation(home, away)

		select {
		case <-ctx.Done():
			return
		default:
		}

		f.cacheMu.Lock()
		f.cache = &predictionCache{
			Home:      home,
			Away:      away,
			Corners:   pred,
			Timestamp: time.Now(),
		}
		f.cacheMu.Unlock()

		fyne.Do(func() {
			select {
			case <-ctx.Done():
				return
			default:
				f.displayCornersResult(pred, home, away)
			}
		})
	}(ctx, home, away)
}
func (f *FyneUI) updateOutcomeAsync(resultLabel *widget.Label) {
	home, away := f.getTeams()
	if home == nil || away == nil {
		resultLabel.SetText("❌ Selecciona ambos equipos primero")
		return
	}

	// Cancelar cálculos previos
	f.cancelCurrentCalculation()
	ctx := f.calcCtx

	resultLabel.SetText("🔄 Calculando probabilidades 1X2...")

	go func(ctx context.Context, home, away *models.Team) {
		defer func() {
			if r := recover(); r != nil {
				fyne.Do(func() {
					f.showError(fmt.Sprintf("Error en cálculo: %v", r))
				})
			}
		}()

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Usar el nuevo método PredictOutcome
		outcome := f.Predictor.PredictOutcome(home, away)

		select {
		case <-ctx.Done():
			return
		default:
		}

		fyne.Do(func() {
			select {
			case <-ctx.Done():
				return
			default:
				texto := fmt.Sprintf(`🏆 PREDICCIÓN DEL RESULTADO

🏠 %s vs ✈️ %s

📊 Probabilidades:
• Victoria Local: %.1f%%
• Empate:        %.1f%%
• Victoria Visitante: %.1f%%

💰 Cuotas justas:
• 1: %.2f
• X: %.2f
• 2: %.2f`,
					home.Name, away.Name,
					outcome.HomeWinProb*100,
					outcome.DrawProb*100,
					outcome.AwayWinProb*100,
					outcome.HomeOdds,
					outcome.DrawOdds,
					outcome.AwayOdds,
				)
				resultLabel.SetText(texto)
			}
		})
	}(ctx, home, away)
}
// displayCornersResult muestra el resultado de corners
func (f *FyneUI) displayCornersResult(pred *models.CornerPrediction, home, away *models.Team) {
	texto := fmt.Sprintf(`📊 PREDICCIÓN DE CORNERS

🏠 %s: %.2f corners esperados
✈️ %s: %.2f corners esperados
📈 Total esperado: %.2f corners
📉 Varianza total: %.2f
🎯 Desviación típica: %.2f
🎲 Rango 68%%: [%.0f – %.0f]

📊 Probabilidades acumuladas:
• Over 7.5:  %.1f%%
• Over 8.5:  %.1f%%
• Over 9.5:  %.1f%%
• Over 10.5: %.1f%%`,
		home.Name, pred.HomeExpected,
		away.Name, pred.AwayExpected,
		pred.TotalExpected,
		pred.TotalVariance,
		pred.StdDev,
		pred.TotalExpected-pred.StdDev, pred.TotalExpected+pred.StdDev,
		pred.Over75*100, pred.Over85*100, pred.Over95*100, pred.Over105*100,
	)

	f.resultText.SetText(texto)

	var distTexto strings.Builder
	distTexto.WriteString("📊 Distribución de probabilidad:\n\n")

	const maxBar = 40
	for total := 0; total <= 20; total++ {
		if prob, exists := pred.Distribution[total]; exists && prob > 0.001 {
			barLength := int(prob * maxBar)
			if barLength < 1 {
				barLength = 1
			}
			barra := strings.Repeat("█", barLength)
			distTexto.WriteString(fmt.Sprintf("%2d: %s (%.1f%%)\n", total, barra, prob*100))
		}
	}
	f.cornersChart.SetText(distTexto.String())
}

// updateBTTSAsync calcula BTTS en segundo plano
func (f *FyneUI) updateBTTSAsync() {
	home, away := f.getTeams()
	if home == nil || away == nil {
		f.bttsResult.SetText("❌ Selecciona ambos equipos primero")
		return
	}

	f.cacheMu.RLock()
	if f.cache != nil && f.cache.Home == home && f.cache.Away == away &&
		f.cache.BTTS != nil && time.Since(f.cache.Timestamp) < 5*time.Minute {
		f.displayBTTSResult(f.cache.BTTS, home, away)
		f.cacheMu.RUnlock()
		return
	}
	f.cacheMu.RUnlock()

	f.cancelCurrentCalculation()
	ctx := f.calcCtx

	f.bttsResult.SetText("🔄 Calculando probabilidad BTTS...")

	go func(ctx context.Context, home, away *models.Team) {
		defer func() {
			if r := recover(); r != nil {
				fyne.Do(func() {
					f.showError(fmt.Sprintf("Error en cálculo BTTS: %v", r))
				})
			}
		}()

		select {
		case <-ctx.Done():
			return
		default:
		}

		pred := f.Predictor.CalculateBTTSProbability(home, away)

		select {
		case <-ctx.Done():
			return
		default:
		}

		f.cacheMu.Lock()
		if f.cache != nil && f.cache.Home == home && f.cache.Away == away {
			f.cache.BTTS = pred
			f.cache.Timestamp = time.Now()
		}
		f.cacheMu.Unlock()

		fyne.Do(func() {
			select {
			case <-ctx.Done():
				return
			default:
				f.displayBTTSResult(pred, home, away)
			}
		})
	}(ctx, home, away)
}

// displayBTTSResult muestra el resultado de BTTS
func (f *FyneUI) displayBTTSResult(pred *models.BTTSPrediction, home, away *models.Team) {
	texto := fmt.Sprintf(`⚽ PREDICCIÓN BTTS

🏠 %s vs ✈️ %s

📈 Probabilidad de BTTS (SÍ): %.1f%%
💰 Cuota justa SÍ: %.2f
💰 Cuota justa NO: %.2f`,
		home.Name, away.Name,
		pred.Probability*100,
		pred.YesOdds,
		1/(1-pred.Probability),
	)

	f.bttsResult.SetText(texto)
}

// updateHandicapAsync calcula handicap en segundo plano
func (f *FyneUI) updateHandicapAsync(handicap float64) {
	home, away := f.getTeams()
	if home == nil || away == nil {
		f.handicapResult.SetText("❌ Selecciona ambos equipos primero")
		return
	}

	f.cancelCurrentCalculation()
	ctx := f.calcCtx

	f.handicapResult.SetText("🔄 Calculando handicap...")

	go func(ctx context.Context, home, away *models.Team, h float64) {
		defer func() {
			if r := recover(); r != nil {
				fyne.Do(func() {
					f.showError(fmt.Sprintf("Error en cálculo handicap: %v", r))
				})
			}
		}()

		select {
		case <-ctx.Done():
			return
		default:
		}

		pred := f.Predictor.CalculateHandicap(home, away, h)

		select {
		case <-ctx.Done():
			return
		default:
		}

		fyne.Do(func() {
			select {
			case <-ctx.Done():
				return
			default:
				f.displayHandicapResult(pred, home, away)
			}
		})
	}(ctx, home, away, handicap)
}

// displayHandicapResult muestra el resultado de handicap
func (f *FyneUI) displayHandicapResult(pred *models.HandicapPrediction, home, away *models.Team) {
	texto := fmt.Sprintf(`🎯 HANDICAP ASIÁTICO

🏠 %s vs ✈️ %s
Handicap: %.2f

📊 Probabilidades:
• Local cubre: %.1f%%
• Visitante cubre: %.1f%%

💰 Cuotas justas:
• Local: %.2f
• Visitante: %.2f

⚽ Diferencia esperada: %.2f goles`,
		home.Name, away.Name,
		pred.Handicap,
		pred.HomeProb*100,
		pred.AwayProb*100,
		1/pred.HomeProb,
		1/pred.AwayProb,
		pred.ExpectedValue,
	)

	f.handicapResult.SetText(texto)
}

// calculateKelly calcula y muestra resultado de Kelly
func (f *FyneUI) calculateKelly() {
	oddsText := f.oddsEntry.Text
	if oddsText == "" {
		f.kellyResult.SetText("❌ Ingresa una cuota")
		return
	}

	odds, err := strconv.ParseFloat(oddsText, 64)
	if err != nil || odds <= 1.0 {
		f.kellyResult.SetText("❌ Cuota inválida (debe ser > 1.0)")
		return
	}

	prob, _ := f.probBinding.Get()
	prob = prob / 100.0

	kelly := f.Predictor.KellyCriterion(prob, odds)
	edge := f.Predictor.Edge(prob, odds)

	var recomendacion string
	if kelly > 0.05 {
		recomendacion = "✅ APOSTAR - Valor significativo"
	} else if kelly > 0 {
		recomendacion = "⚠️ APOSTAR - Valor marginal"
	} else {
		recomendacion = "❌ NO APOSTAR - Sin valor"
	}

	texto := fmt.Sprintf(`📊 RESULTADOS KELLY

Probabilidad: %.1f%%
Cuota: %.2f
Edge: %+.2f%%
Kelly óptimo: %.2f%%

%s`,
		prob*100, odds, edge, kelly*100,
		recomendacion,
	)

	f.kellyResult.SetText(texto)
}

// showError muestra un diálogo de error
func (f *FyneUI) showError(msg string) {
	dialog.ShowError(fmt.Errorf("%s", msg), f.Window)
}
