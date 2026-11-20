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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type predictionCache struct {
	Home      *models.Team
	Away      *models.Team
	Corners   *models.CornerPrediction
	BTTS      *models.BTTSPrediction
	Outcome   *models.OutcomePrediction
	Timestamp time.Time
}

type TeamSelector struct {
	entry      *widget.Entry
	select_    *widget.Select
	container  *fyne.Container
	onSelected func(team *models.Team)
}

type FyneUI struct {
	Predictor *models.Predictor
	Window    fyne.Window
	App       fyne.App
	DataDir   string
    MostrarAlertaPermiso   bool 

	homeSelector *TeamSelector
	awaySelector *TeamSelector

	// Labels de Córners
	homeExpectedText  *canvas.Text
	awayExpectedText  *canvas.Text
	totalExpectedText *canvas.Text
	over75Label       *widget.Label
	over85Label       *widget.Label
	over95Label       *widget.Label
	over105Label      *widget.Label

	// Labels de 1X2 y xG
	homeWinText *canvas.Text
	drawText    *canvas.Text
	awayWinText *canvas.Text
	homeOdds    *widget.Label
	drawOdds    *widget.Label
	awayOdds    *widget.Label
	homeXGLabel *widget.Label
	awayXGLabel *widget.Label

	// BTTS y Handicap
	bttsResult     *widget.Label
	handicapResult *widget.Label
	handicapEntry  *widget.Entry

	mu          sync.RWMutex
	currentHome *models.Team
	currentAway *models.Team

	calcCtx    context.Context
	calcCancel context.CancelFunc
	calcMu     sync.Mutex

	cache   *predictionCache
	cacheMu sync.RWMutex
}

func NewFyneUI(predictor *models.Predictor, dataDir string) *FyneUI {
	myApp := app.NewWithID("com.futcor.predictor")
	myApp.Settings().SetTheme(&DarkSoccerTheme{})
	myWindow := myApp.NewWindow("Futbol Predictor")
	myWindow.Resize(fyne.NewSize(400, 800)) // Tamaño vertical para móvil

	return &FyneUI{
		Predictor: predictor,
		App:       myApp,
		Window:    myWindow,
		DataDir:   dataDir,
	}
}

func (f *FyneUI) Run() {
	f.Window.SetContent(f.buildMainUI())
	// Comprobar si debemos mostrar el diálogo de permisoiso
	if f.MostrarAlertaPermiso {
		dialog.ShowCustom("⚠️ Permiso Requerido", "Entendido", 
			widget.NewLabel("Para descargar los datos de fútbol, ve a Ajustes > Aplicaciones > Futcor Predict > Permisos y activa 'Acceso a todos los archivos'.\n\nLuego reinicia la app."), 
			f.Window)
	}
	
	f.Window.ShowAndRun()
}

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

// --- CONSTRUCCIÓN DE LA UI ---

func (f *FyneUI) buildMainUI() fyne.CanvasObject {
	// 1. Selectores de equipo
	f.homeSelector = f.createTeamSelector("Local", true)
	f.homeSelector.onSelected = func(t *models.Team) { f.onTeamSelected() }
	f.awaySelector = f.createTeamSelector("Visitante", false)
	f.awaySelector.onSelected = func(t *models.Team) { f.onTeamSelected() }

	// 2. Tarjeta superior
	headerCard := container.NewPadded(f.buildHeaderCard())

	// 3. Pestañas (Abajo) usando íconos integrados de theme
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("1X2", theme.ConfirmIcon(), f.buildOutcomeTab()),
		container.NewTabItemWithIcon("Corners", theme.GridIcon(), f.buildCornersTab()),
		container.NewTabItemWithIcon("BTTS", theme.InfoIcon(), f.buildBTTSTab()),
		container.NewTabItemWithIcon("Handicap", theme.MenuIcon(), f.buildHandicapTab()),
	)
	tabs.SetTabLocation(container.TabLocationBottom)

	return container.NewBorder(headerCard, nil, nil, nil, tabs)
}

func (f *FyneUI) buildHeaderCard() fyne.CanvasObject {
	swapBtn := widget.NewButtonWithIcon("", theme.MediaFastForwardIcon(), func() {
		h := f.homeSelector.entry.Text
		a := f.awaySelector.entry.Text
		f.homeSelector.entry.SetText(a)
		f.awaySelector.entry.SetText(h)
		f.handleSearch()
	})

	selectorGrid := container.NewBorder(
		nil,
		nil,
		nil,
		nil,
		container.NewGridWithColumns(3,
			f.homeSelector.container,
			container.NewCenter(swapBtn),
			f.awaySelector.container,
		),
	)

	importBtn := widget.NewButtonWithIcon("Actualizar Data", theme.DownloadIcon(), func() {
		dialog.ShowInformation("Info", "Módulo de descarga en construcción...", f.Window)
	})

	titleLabel := widget.NewLabelWithStyle("PARTIDO", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	titleWithIcon := container.NewHBox(widget.NewIcon(theme.SearchIcon()), titleLabel)
	headerTop := container.NewBorder(nil, nil, titleWithIcon, importBtn)

	content := container.NewPadded(container.NewVBox(
		headerTop,
		widget.NewSeparator(),
		selectorGrid,
	))

	return widget.NewCard("", "", content)
}

func (f *FyneUI) buildOutcomeTab() fyne.CanvasObject {
	f.homeWinText = canvas.NewText("0.0%", colorAccentGreen)
	f.homeWinText.TextSize = 32
	f.homeWinText.TextStyle = fyne.TextStyle{Bold: true}
	f.homeWinText.Alignment = fyne.TextAlignCenter

	f.drawText = canvas.NewText("0.0%", colorAccentGold)
	f.drawText.TextSize = 32
	f.drawText.TextStyle = fyne.TextStyle{Bold: true}
	f.drawText.Alignment = fyne.TextAlignCenter

	f.awayWinText = canvas.NewText("0.0%", colorAccentBlue)
	f.awayWinText.TextSize = 32
	f.awayWinText.TextStyle = fyne.TextStyle{Bold: true}
	f.awayWinText.Alignment = fyne.TextAlignCenter

	f.homeOdds = widget.NewLabelWithStyle("Cuota\n-", fyne.TextAlignCenter, fyne.TextStyle{})
	f.drawOdds = widget.NewLabelWithStyle("Cuota\n-", fyne.TextAlignCenter, fyne.TextStyle{})
	f.awayOdds = widget.NewLabelWithStyle("Cuota\n-", fyne.TextAlignCenter, fyne.TextStyle{})

	homeTitle := canvas.NewText("1 · LOCAL", colorTextPrimary)
	homeTitle.TextSize = 13
	homeTitle.TextStyle = fyne.TextStyle{Bold: true}
	homeTitle.Alignment = fyne.TextAlignCenter

	drawTitle := canvas.NewText("X · EMPATE", colorTextPrimary)
	drawTitle.TextSize = 13
	drawTitle.TextStyle = fyne.TextStyle{Bold: true}
	drawTitle.Alignment = fyne.TextAlignCenter

	awayTitle := canvas.NewText("2 · VISITANTE", colorTextPrimary)
	awayTitle.TextSize = 13
	awayTitle.TextStyle = fyne.TextStyle{Bold: true}
	awayTitle.Alignment = fyne.TextAlignCenter

	cardHome := widget.NewCard("", "", container.NewVBox(
		container.NewCenter(homeTitle),
		container.NewCenter(f.homeWinText),
		f.homeOdds,
	))
	cardDraw := widget.NewCard("", "", container.NewVBox(
		container.NewCenter(drawTitle),
		container.NewCenter(f.drawText),
		f.drawOdds,
	))
	cardAway := widget.NewCard("", "", container.NewVBox(
		container.NewCenter(awayTitle),
		container.NewCenter(f.awayWinText),
		f.awayOdds,
	))

	grid := container.NewGridWithColumns(3, cardHome, cardDraw, cardAway)

	f.homeXGLabel = widget.NewLabelWithStyle("Local\n0.00 xG", fyne.TextAlignCenter, fyne.TextStyle{})
	f.homeXGLabel.Wrapping = fyne.TextWrapWord
	f.awayXGLabel = widget.NewLabelWithStyle("Visitante\n0.00 xG", fyne.TextAlignCenter, fyne.TextStyle{})
	f.awayXGLabel.Wrapping = fyne.TextWrapWord

	xgTitle := widget.NewLabelWithStyle("GOLES ESPERADOS (xG)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	xgCard := widget.NewCard("", "", container.NewPadded(container.NewVBox(
		xgTitle,
		widget.NewSeparator(),
		container.NewGridWithColumns(2, f.homeXGLabel, f.awayXGLabel),
	)))

	calcBtn := widget.NewButtonWithIcon("Calcular 1X2", theme.ViewRefreshIcon(), func() {
		f.updateOutcomeAsync()
	})
	calcBtn.Importance = widget.HighImportance

	content := container.NewPadded(container.NewVBox(
		grid,
		xgCard,
		widget.NewSeparator(),
		calcBtn,
	))

	return container.NewVScroll(content)
}

func (f *FyneUI) buildCornersTab() fyne.CanvasObject {
	f.homeExpectedText = canvas.NewText("0.00", colorTextPrimary)
	f.homeExpectedText.TextSize = 26
	f.homeExpectedText.TextStyle = fyne.TextStyle{Bold: true}
	f.homeExpectedText.Alignment = fyne.TextAlignCenter

	f.awayExpectedText = canvas.NewText("0.00", colorTextPrimary)
	f.awayExpectedText.TextSize = 26
	f.awayExpectedText.TextStyle = fyne.TextStyle{Bold: true}
	f.awayExpectedText.Alignment = fyne.TextAlignCenter

	f.totalExpectedText = canvas.NewText("0.00", colorTextPrimary)
	f.totalExpectedText.TextSize = 36
	f.totalExpectedText.TextStyle = fyne.TextStyle{Bold: true}
	f.totalExpectedText.Alignment = fyne.TextAlignCenter

	f.over75Label = widget.NewLabelWithStyle("-", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	f.over85Label = widget.NewLabelWithStyle("-", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	f.over95Label = widget.NewLabelWithStyle("-", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	f.over105Label = widget.NewLabelWithStyle("-", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	homeTitle := widget.NewLabelWithStyle("LOCAL", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	homeSub := widget.NewLabelWithStyle("Corners esperados", fyne.TextAlignCenter, fyne.TextStyle{})
	homeSub.Wrapping = fyne.TextWrapWord

	awayTitle := widget.NewLabelWithStyle("VISITANTE", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	awaySub := widget.NewLabelWithStyle("Corners esperados", fyne.TextAlignCenter, fyne.TextStyle{})
	awaySub.Wrapping = fyne.TextWrapWord

	cardHome := widget.NewCard("", "", container.NewPadded(container.NewVBox(
		homeTitle,
		homeSub,
		container.NewCenter(f.homeExpectedText),
	)))
	cardAway := widget.NewCard("", "", container.NewPadded(container.NewVBox(
		awayTitle,
		awaySub,
		container.NewCenter(f.awayExpectedText),
	)))
	gridEquipos := container.NewGridWithColumns(2, cardHome, cardAway)

	totalTitle := widget.NewLabelWithStyle("TOTAL DEL PARTIDO", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	totalSub := widget.NewLabelWithStyle("Corners totales esperados", fyne.TextAlignCenter, fyne.TextStyle{})
	totalCard := widget.NewCard("", "", container.NewPadded(container.NewVBox(
		totalTitle,
		totalSub,
		container.NewCenter(f.totalExpectedText),
	)))

	overTitle := widget.NewLabelWithStyle("PROBABILIDAD OVER", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	gridProbs := container.NewGridWithColumns(4,
		container.NewVBox(widget.NewLabelWithStyle("+7.5", fyne.TextAlignCenter, fyne.TextStyle{}), f.over75Label),
		container.NewVBox(widget.NewLabelWithStyle("+8.5", fyne.TextAlignCenter, fyne.TextStyle{}), f.over85Label),
		container.NewVBox(widget.NewLabelWithStyle("+9.5", fyne.TextAlignCenter, fyne.TextStyle{}), f.over95Label),
		container.NewVBox(widget.NewLabelWithStyle("+10.5", fyne.TextAlignCenter, fyne.TextStyle{}), f.over105Label),
	)
	overCard := widget.NewCard("", "", container.NewPadded(container.NewVBox(
		overTitle,
		widget.NewSeparator(),
		gridProbs,
	)))

	calcBtn := widget.NewButtonWithIcon("Calcular", theme.ViewRefreshIcon(), func() {
		f.updateCornersAsync()
	})
	calcBtn.Importance = widget.HighImportance

	content := container.NewPadded(container.NewVBox(
		gridEquipos,
		totalCard,
		overCard,
		widget.NewSeparator(),
		calcBtn,
	))

	return container.NewVScroll(content)
}

func (f *FyneUI) buildBTTSTab() fyne.CanvasObject {
	f.bttsResult = widget.NewLabel("Selecciona equipos para predecir BTTS")
	f.bttsResult.Wrapping = fyne.TextWrapWord

	resultCard := widget.NewCard(
		"AMBOS MARCAN (BTTS)",
		"Probabilidad y cuotas justas",
		container.NewPadded(f.bttsResult),
	)

	calcBtn := widget.NewButtonWithIcon("Calcular BTTS", theme.ViewRefreshIcon(), func() {
		f.updateBTTSAsync()
	})
	calcBtn.Importance = widget.HighImportance

	bottomBar := container.NewPadded(container.NewVBox(
		widget.NewSeparator(),
		calcBtn,
	))

	return container.NewBorder(
		nil,
		bottomBar,
		nil,
		nil,
		container.NewVScroll(container.NewPadded(resultCard)),
	)
}

func (f *FyneUI) buildHandicapTab() fyne.CanvasObject {
	f.handicapResult = widget.NewLabel("Selecciona equipos para predecir Handicap")
	f.handicapResult.Wrapping = fyne.TextWrapWord
	f.handicapEntry = widget.NewEntry()
	f.handicapEntry.SetText("-0.5")

	topInput := container.NewPadded(container.NewVBox(
		widget.NewLabelWithStyle("Handicap Asiático:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		f.handicapEntry,
	))

	resultCard := widget.NewCard(
		"HANDICAP",
		"Probabilidad de cobertura",
		container.NewPadded(f.handicapResult),
	)

	calcBtn := widget.NewButtonWithIcon("Calcular Handicap", theme.ViewRefreshIcon(), func() {
		h, _ := strconv.ParseFloat(f.handicapEntry.Text, 64)
		f.updateHandicapAsync(h)
	})
	calcBtn.Importance = widget.HighImportance

	bottomBar := container.NewPadded(container.NewVBox(
		widget.NewSeparator(),
		calcBtn,
	))

	return container.NewBorder(
		topInput,
		bottomBar,
		nil,
		nil,
		container.NewVScroll(container.NewPadded(resultCard)),
	)
}

// --- LÓGICA DE SELECCIÓN Y CÁLCULOS ---

func (f *FyneUI) createTeamSelector(placeholder string, isHome bool) *TeamSelector {
	ts := &TeamSelector{}
	ts.entry = widget.NewEntry()
	ts.entry.SetPlaceHolder(placeholder)
	ts.select_ = widget.NewSelect([]string{}, func(selected string) {
		if selected == "" {
			return
		}
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
		ts.select_.ClearSelected()
		ts.select_.Options = []string{}
		ts.select_.Refresh()
		ts.select_.Hide()
	})
	ts.select_.Hide()
	ts.container = container.NewVBox(ts.entry, ts.select_)

	var lastSearch string
	ts.entry.OnChanged = func(text string) {
		if len(text) < 2 {
			ts.select_.Hide()
			ts.select_.Options = []string{}
			return
		}
		if text == lastSearch {
			return
		}
		lastSearch = text
		var matches []string
		for _, team := range f.Predictor.Teams {
			if strings.Contains(strings.ToLower(team.Name), strings.ToLower(text)) {
				matches = append(matches, team.Name)
			}
		}
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

func (f *FyneUI) onTeamSelected() {
	home, away := f.getTeams()
	if home != nil && away != nil && home.Name != away.Name {
		f.updateOutcomeAsync()
		f.updateCornersAsync()
		f.updateBTTSAsync()
	}
}

func (f *FyneUI) handleSearch() {
	homeText, awayText := f.homeSelector.entry.Text, f.awaySelector.entry.Text
	if homeText == "" || awayText == "" {
		return
	}
	var home, away *models.Team
	for _, team := range f.Predictor.Teams {
		if strings.EqualFold(team.Name, homeText) {
			home = team
		}
		if strings.EqualFold(team.Name, awayText) {
			away = team
		}
	}
	if home != nil && away != nil {
		f.setTeams(home, away)
		f.onTeamSelected()
	}
}

func (f *FyneUI) updateOutcomeAsync() {
	home, away := f.getTeams()
	if home == nil || away == nil {
		return
	}
	go func() {
		outcome := f.Predictor.PredictOutcome(home, away)
		homeXG, awayXG := calculateExpectedGoalsForDisplay(home, away)
		fyne.Do(func() {
			f.homeWinText.Text = fmt.Sprintf("%.1f%%", outcome.HomeWinProb*100)
			f.drawText.Text = fmt.Sprintf("%.1f%%", outcome.DrawProb*100)
			f.awayWinText.Text = fmt.Sprintf("%.1f%%", outcome.AwayWinProb*100)
			f.homeWinText.Refresh()
			f.drawText.Refresh()
			f.awayWinText.Refresh()

			f.homeOdds.SetText(fmt.Sprintf("Cuota\n%.2f", outcome.HomeOdds))
			f.drawOdds.SetText(fmt.Sprintf("Cuota\n%.2f", outcome.DrawOdds))
			f.awayOdds.SetText(fmt.Sprintf("Cuota\n%.2f", outcome.AwayOdds))

			f.homeXGLabel.SetText(fmt.Sprintf("%s\n%.2f xG", home.Name, homeXG))
			f.awayXGLabel.SetText(fmt.Sprintf("%s\n%.2f xG", away.Name, awayXG))
		})
	}()
}

func (f *FyneUI) updateCornersAsync() {
	home, away := f.getTeams()
	if home == nil || away == nil {
		return
	}
	go func() {
		pred := f.Predictor.CalculateCornerExpectation(home, away)
		fyne.Do(func() {
			f.displayCornersResult(pred, home, away)
		})
	}()
}

func (f *FyneUI) displayCornersResult(pred *models.CornerPrediction, home, away *models.Team) {
	f.homeExpectedText.Text = fmt.Sprintf("%.2f", pred.HomeExpected)
	f.awayExpectedText.Text = fmt.Sprintf("%.2f", pred.AwayExpected)
	f.totalExpectedText.Text = fmt.Sprintf("%.2f", pred.TotalExpected)

	if pred.TotalExpected > 10.5 {
		f.totalExpectedText.Color = colorAccentGreen
	} else {
		f.totalExpectedText.Color = colorTextPrimary
	}

	f.homeExpectedText.Refresh()
	f.awayExpectedText.Refresh()
	f.totalExpectedText.Refresh()

	f.over75Label.SetText(fmt.Sprintf("%.1f%%", pred.Over75*100))
	f.over85Label.SetText(fmt.Sprintf("%.1f%%", pred.Over85*100))
	f.over95Label.SetText(fmt.Sprintf("%.1f%%", pred.Over95*100))
	f.over105Label.SetText(fmt.Sprintf("%.1f%%", pred.Over105*100))
}

func (f *FyneUI) updateBTTSAsync() {
	home, away := f.getTeams()
	if home == nil || away == nil {
		return
	}
	go func() {
		pred := f.Predictor.CalculateBTTSProbability(home, away)
		fyne.Do(func() {
			f.bttsResult.SetText(fmt.Sprintf("Probabilidad BTTS: %.1f%%\nCuota SÍ: %.2f\nCuota NO: %.2f", pred.Probability*100, pred.YesOdds, 1/(1-pred.Probability)))
		})
	}()
}

func (f *FyneUI) updateHandicapAsync(h float64) {
	home, away := f.getTeams()
	if home == nil || away == nil {
		return
	}
	go func() {
		pred := f.Predictor.CalculateHandicap(home, away, h)
		fyne.Do(func() {
			f.handicapResult.SetText(fmt.Sprintf("Local cubre: %.1f%%\nVisitante cubre: %.1f%%\nDiferencia: %.2f", pred.HomeProb*100, pred.AwayProb*100, pred.ExpectedValue))
		})
	}()
}

// calculateExpectedGoalsForDisplay calcula los xG esperados para mostrar en la pestaña 1X2
// usando únicamente campos exportados de models.Team sin alterar models/fut.go.
func calculateExpectedGoalsForDisplay(home, away *models.Team) (float64, float64) {
	homeXGFor := home.XGForHome
	homeXGAgainst := home.XGAgainstHome
	homeFinDelta := home.FinishingDeltaHome

	awayXGFor := away.XGForAway
	awayXGAgainst := away.XGAgainstAway
	awayFinDelta := away.FinishingDeltaAway

	if homeXGFor == 0 {
		homeXGFor = home.GoalsForAvg * 0.85
	}
	if awayXGFor == 0 {
		awayXGFor = away.GoalsForAvg * 0.85
	}
	if homeXGAgainst == 0 {
		homeXGAgainst = home.GoalsAgainstAvg * 0.85
	}
	if awayXGAgainst == 0 {
		awayXGAgainst = away.GoalsAgainstAvg * 0.85
	}

	isChampions := home.Competition == "Champions" && away.Competition == "Champions"
	homeLocalFactor := 1.1
	awayAwayFactor := 0.95
	if isChampions {
		homeLocalFactor = 1.05
		awayAwayFactor = 0.98
		homeXGFor *= 1.08
		awayXGFor *= 1.08
	}

	avgXG := 1.35
	if home.Competition == "Champions" {
		avgXG = 1.50
	} else {
		leagueAvg := map[string]float64{
			"LaLiga":           1.35,
			"Premier League":   1.45,
			"Bundesliga":       1.40,
			"Serie A":          1.30,
			"Ligue 1":          1.25,
			"Champions League": 1.50,
		}
		if val, ok := leagueAvg[home.League]; ok {
			avgXG = val
		}
	}

	homeDefQuality := homeXGAgainst / avgXG
	awayDefQuality := awayXGAgainst / avgXG

	homeOffEfficiency := 1.0
	if homeFinDelta > 0.3 {
		homeOffEfficiency = 0.95
	} else if homeFinDelta < -0.3 {
		homeOffEfficiency = 1.05
	}
	awayOffEfficiency := 1.0
	if awayFinDelta > 0.3 {
		awayOffEfficiency = 0.95
	} else if awayFinDelta < -0.3 {
		awayOffEfficiency = 1.05
	}

	strengthFactor := 1.0 + (home.PowerRating-away.PowerRating)/200

	homeExpected := homeXGFor * homeOffEfficiency * strengthFactor * homeLocalFactor * awayDefQuality
	awayExpected := awayXGFor * awayOffEfficiency * (1 / strengthFactor) * awayAwayFactor * homeDefQuality

	return homeExpected, awayExpected
}
