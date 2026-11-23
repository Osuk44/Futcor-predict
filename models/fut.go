package models

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// CornerStats estadísticas de corners de un equipo
type CornerStats struct {
	MatchesPlayed      int
	AvgCornersTaken    float64
	AvgCornersConceded float64
	OverPercent        map[float64]float64 // Ej: 7.5 -> 0.92 (92%)
	UnderPercent       map[float64]float64
}

// Team representa un equipo con todas sus métricas
type Team struct {
	ID          string
	Name        string
	League      string
	PowerRating float64
	GAx         float64
	GSAx        float64
	Style       string
	Competition string

	// Datos de corners (de CSVs específicos)
	HomeCorners *CornerStats
	AwayCorners *CornerStats

	// Datos de BTTS (de Footiqo)
	BTTS1H      float64 // BTTS primera parte
	BTTS2H      float64 // BTTS segunda parte
	BTTSOverall float64 // BTTS partido completo

	// Datos de WDL (de Footiqo)
	WinHome  float64
	DrawHome float64
	LossHome float64
	WinAway  float64
	DrawAway float64
	LossAway float64

	// Datos originales (para compatibilidad con código existente)
	AvgCornersFor     float64
	AvgCornersAgainst float64
	CornerVariance    float64
	BTTSRate          float64
	GoalsForAvg       float64
	GoalsAgainstAvg   float64
	HandicapStats     map[float64]float64

	// xG separado por local/visitante
	XGForHome          float64
	XGAgainstHome      float64
	XGForAway          float64
	XGAgainstAway      float64
	FinishingDeltaHome float64
	FinishingDeltaAway float64
	ConcededDeltaHome  float64
	ConcededDeltaAway  float64
}

// Match representa un partido a predecir
type Match struct {
	HomeTeam    *Team
	AwayTeam    *Team
	HomeCorners int
	AwayCorners int
	HomeGoals   int
	AwayGoals   int
	BTTS        bool
	Odds        map[string]float64 // Cuotas reales de la casa
}

// NegativeBinomialParams parametros para distribucion binomial negativa
type NegativeBinomialParams struct {
	R float64 // Numero de exitos
	P float64 // Probabilidad de exito
}

// BivariateBinomialParams parametros para binomial bivariada (BTTS)
type BivariateBinomialParams struct {
	P11 float64 // Probabilidad ambos marcan
	P10 float64 // Probabilidad solo local marca
	P01 float64 // Probabilidad solo visita marca
	P00 float64 // Probabilidad ninguno marca
}

// CornerPrediction prediccion de corners
type CornerPrediction struct {
	HomeExpected  float64
	AwayExpected  float64
	TotalExpected float64
	TotalVariance float64
	StdDev        float64
	Distribution  map[int]float64 // Probabilidad por total de corners
	Over75        float64         // Probabilidad de total > 7.5
	Over85        float64         // > 8.5
	Over95        float64         // > 9.5
	Over105       float64         // > 10.5
}

// BTTSPrediction prediccion de Both Teams To Score
type BTTSPrediction struct {
	Probability float64
	YesOdds     float64
	NoOdds      float64
}

// HandicapPrediction prediccion de handicap
type HandicapPrediction struct {
	Handicap      float64
	HomeProb      float64
	AwayProb      float64
	ExpectedValue float64
}

// OutcomePrediction contiene las probabilidades para 1X2 y las cuotas justas
type OutcomePrediction struct {
	HomeWinProb float64
	DrawProb    float64
	AwayWinProb float64
	HomeOdds    float64
	DrawOdds    float64
	AwayOdds    float64
}

// Predictor modelo principal
type Predictor struct {
	Teams   map[string]*Team
	Matches []*Match
}

// NewPredictor crea una nueva instancia del predictor
func NewPredictor() *Predictor {
	return &Predictor{
		Teams:   make(map[string]*Team),
		Matches: make([]*Match, 0),
	}
}

// --- Helpers internos de parseo y normalización ---

func parseCleanFloat(s string) (float64, error) {
	cleaned := strings.TrimSpace(s)
	cleaned = strings.TrimSuffix(cleaned, "%")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" || cleaned == "-" || strings.EqualFold(cleaned, "N/A") {
		return 0, fmt.Errorf("valor vacío")
	}
	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, fmt.Errorf("valor no finito")
	}
	return val, nil
}

func parsePercentProportion(s string) (float64, error) {
	val, err := parseCleanFloat(s)
	if err != nil {
		return 0, err
	}
	prop := val / 100.0
	if prop < 0 {
		prop = 0
	}
	if prop > 1 {
		prop = 1
	}
	return prop, nil
}

// parseFlexibleProportion acepta tanto proporción (0.68) como porcentaje (68 o 68%)
func parseFlexibleProportion(s string) (float64, error) {
	raw := strings.TrimSpace(s)
	val, err := parseCleanFloat(raw)
	if err != nil {
		return 0, err
	}
	if strings.Contains(raw, "%") || val > 1.0 {
		val /= 100.0
	}
	return clampFloat(val, 0, 1), nil
}

func parseCleanInt(s string) (int, error) {
	cleaned := strings.TrimSpace(s)
	if cleaned == "" || cleaned == "-" {
		return 0, fmt.Errorf("valor vacío")
	}
	if idx := strings.Index(cleaned, "."); idx > 0 {
		if f, err := strconv.ParseFloat(cleaned, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return int(math.Round(f)), nil
		}
	}
	return strconv.Atoi(cleaned)
}

func isEmptyRecord(record []string) bool {
	for _, cell := range record {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func normalizeHeaderName(h string) string {
	s := strings.ToLower(strings.TrimSpace(h))
	s = strings.TrimPrefix(s, "\ufeff")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.Join(strings.Fields(s), " ")
}

func mapHeaders(header []string) map[string]int {
	idxMap := make(map[string]int, len(header))
	for i, col := range header {
		key := normalizeHeaderName(col)
		if key != "" {
			if _, exists := idxMap[key]; !exists {
				idxMap[key] = i
			}
		}
	}
	return idxMap
}

func lookupHeader(headers map[string]int, keys ...string) (int, bool) {
	for _, k := range keys {
		if idx, ok := headers[k]; ok {
			return idx, true
		}
	}
	return -1, false
}

var diacriticReplacer = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ñ", "n", "ç", "c", "ý", "y", "ÿ", "y", "ß", "ss",
	"š", "s", "ž", "z", "č", "c", "ř", "r", "ł", "l", "ń", "n",
)

func normalizeTeamName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = diacriticReplacer.Replace(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		if unicode.IsSpace(r) || r == '-' || r == '_' || r == '.' || r == '/' || r == '&' {
			return ' '
		}
		return -1
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func normalizeLeagueName(league string) string {
	s := strings.ToLower(strings.TrimSpace(league))
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.Join(strings.Fields(s), " ")

	switch {
	case strings.Contains(s, "champions"):
		return "Champions League"
	case strings.Contains(s, "premier league"):
		return "Premier League"
	case strings.Contains(s, "bundesliga"):
		return "Bundesliga"
	case strings.Contains(s, "laliga") || strings.Contains(s, "la liga"):
		return "LaLiga"
	case strings.Contains(s, "serie a betano") || strings.Contains(s, "brazil"):
		return "Brazil Brazilian League"
	case strings.Contains(s, "serie a"):
		return "Serie A"
	case strings.Contains(s, "ligue 1"):
		return "Ligue 1"
	case strings.Contains(s, "liga mx"):
		return "Liga MX"
	default:
		return strings.TrimSpace(league)
	}
}

func isChampionsCompetition(league, competition string) bool {
	comp := strings.ToLower(strings.TrimSpace(competition))
	if strings.Contains(comp, "champions") {
		return true
	}
	lg := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(league), "_", " "))
	return strings.Contains(lg, "champions")
}

func clampFloat(val, minVal, maxVal float64) float64 {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return minVal
	}
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

// LoadTeamsFromCSV carga equipos desde archivo CSV (soporta 13 o 14 columnas y detección por cabecera)
func (p *Predictor) LoadTeamsFromCSV(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}

	headers := mapHeaders(records[0])
	headerCount := len(records[0])

	loadedCount := 0
	skippedCount := 0

	for i, record := range records {
		if i == 0 || isEmptyRecord(record) {
			continue
		}

		if len(record) < 12 {
			fmt.Printf("⚠️ Fila %d saltada: columnas insuficientes (%d)\n", i+1, len(record))
			skippedCount++
			continue
		}

		aligned := len(record) == headerCount
		idxOr := func(defaultIdx int, keys ...string) int {
			if aligned {
				if idx, ok := lookupHeader(headers, keys...); ok {
					return idx
				}
			}
			return defaultIdx
		}

		idIdx := idxOr(0, "id")
		nameIdx := idxOr(1, "nombre", "name", "team", "equipo")
		leagueIdx := idxOr(2, "liga", "league")
		powerIdx := idxOr(3, "powerrating", "power rating")
		gaxIdx := idxOr(4, "gax")
		gsaxIdx := idxOr(5, "gsax")
		styleIdx := idxOr(6, "estilo", "style")
		cornersForIdx := idxOr(7, "avgcornersfor", "avg corners for")
		cornersAgIdx := idxOr(8, "avgcornersagainst", "avg corners against")
		bttsIdx := idxOr(9, "bttsrate", "btts rate")
		gfIdx := idxOr(10, "goalsforavg", "goals for avg")
		gaIdx := idxOr(11, "goalsagainstavg", "goals against avg")

		id := strings.TrimSpace(record[idIdx])
		name := strings.TrimSpace(record[nameIdx])
		league := strings.TrimSpace(record[leagueIdx])
		if id == "" || name == "" {
			fmt.Printf("⚠️ Fila %d saltada: ID o nombre vacío\n", i+1)
			skippedCount++
			continue
		}

		powerRating, err := parseCleanFloat(record[powerIdx])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: PowerRating inválido (%s)\n", i+1, record[powerIdx])
			skippedCount++
			continue
		}

		gax, err := parseCleanFloat(record[gaxIdx])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: GAx inválido (%s)\n", i+1, record[gaxIdx])
			skippedCount++
			continue
		}

		gsax, err := parseCleanFloat(record[gsaxIdx])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: GSAx inválido (%s)\n", i+1, record[gsaxIdx])
			skippedCount++
			continue
		}

		style := strings.ToUpper(strings.TrimSpace(record[styleIdx]))

		avgCornersFor, err := parseCleanFloat(record[cornersForIdx])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: AvgCornersFor inválido (%s)\n", i+1, record[cornersForIdx])
			skippedCount++
			continue
		}

		avgCornersAgainst, err := parseCleanFloat(record[cornersAgIdx])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: AvgCornersAgainst inválido (%s)\n", i+1, record[cornersAgIdx])
			skippedCount++
			continue
		}

		bttsRateProp, err := parseFlexibleProportion(record[bttsIdx])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: BTTSRate inválido (%s)\n", i+1, record[bttsIdx])
			skippedCount++
			continue
		}

		goalsForAvg, err := parseCleanFloat(record[gfIdx])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: GoalsForAvg inválido (%s)\n", i+1, record[gfIdx])
			skippedCount++
			continue
		}

		goalsAgainstAvg, err := parseCleanFloat(record[gaIdx])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: GoalsAgainstAvg inválido (%s)\n", i+1, record[gaIdx])
			skippedCount++
			continue
		}

		// Columnas opcionales (CornerVariance y Competition) en formatos de 13 o 14 columnas
		var cornerVariance float64
		competition := ""

		cvIdx := -1
		compIdx := -1
		if aligned {
			if idx, ok := lookupHeader(headers, "cornervariance", "corner variance"); ok {
				cvIdx = idx
			}
			if idx, ok := lookupHeader(headers, "competition", "competicion"); ok {
				compIdx = idx
			}
		}

		if cvIdx >= 0 && cvIdx < len(record) {
			if cv, err := parseCleanFloat(record[cvIdx]); err == nil && cv > 0 {
				cornerVariance = cv
			}
		}
		if compIdx >= 0 && compIdx < len(record) {
			competition = strings.TrimSpace(record[compIdx])
		}

		// Fallback posicional si la cabecera no identificó las columnas opcionales
		if cvIdx < 0 && compIdx < 0 {
			if len(record) == 13 {
				if cv, err := parseCleanFloat(record[12]); err == nil && cv > 0 {
					cornerVariance = cv
				} else {
					competition = strings.TrimSpace(record[12])
				}
			} else if len(record) >= 14 {
				if cv, err := parseCleanFloat(record[12]); err == nil && cv > 0 {
					cornerVariance = cv
				}
				competition = strings.TrimSpace(record[13])
			}
		}

		if cornerVariance <= 0 && avgCornersFor > 0 {
			cornerVariance = avgCornersFor * 1.5
		}
		if competition == "" && isChampionsCompetition(league, "") {
			competition = "Champions"
		}

		if prev, exists := p.Teams[id]; exists {
			fmt.Printf("⚠️ ID duplicado %q en fila %d: %q sobrescribe a %q\n", id, i+1, name, prev.Name)
		}

		team := &Team{
			ID:                id,
			Name:              name,
			League:            league,
			Competition:       competition,
			PowerRating:       powerRating,
			GAx:               gax,
			GSAx:              gsax,
			Style:             style,
			AvgCornersFor:     avgCornersFor,
			AvgCornersAgainst: avgCornersAgainst,
			CornerVariance:    cornerVariance,
			BTTSRate:          bttsRateProp,
			BTTSOverall:       bttsRateProp,
			GoalsForAvg:       goalsForAvg,
			GoalsAgainstAvg:   goalsAgainstAvg,
			HandicapStats:     make(map[float64]float64),
			HomeCorners:       nil,
			AwayCorners:       nil,
		}

		p.Teams[team.ID] = team
		loadedCount++
	}

	fmt.Printf("✅ Carga completada: %d equipos cargados, %d saltados por errores.\n", loadedCount, skippedCount)
	return nil
}

// CalculateCornerExpectation calcula corners esperados cruzando ataque y defensa rival (estilo Dixon-Coles)
func (p *Predictor) CalculateCornerExpectation(home, away *Team) *CornerPrediction {
	if home == nil || away == nil {
		return &CornerPrediction{Distribution: make(map[int]float64)}
	}

	// Factor de estilo centrado en 1.0 para no inflar promedios que ya reflejan el estilo del equipo
	getStyleFactor := func(style string) float64 {
		factors := map[string]float64{
			"BANDA":    1.06,
			"WINGPLAY": 1.05,
			"MIXTO":    1.00,
			"CENTRO":   0.94,
		}
		if val, ok := factors[strings.ToUpper(strings.TrimSpace(style))]; ok {
			return val
		}
		return 1.0
	}

	homeStyleFactor := getStyleFactor(home.Style)
	awayStyleFactor := getStyleFactor(away.Style)

	strengthFactor := clampFloat(1.0+(home.PowerRating-away.PowerRating)/150.0, 0.82, 1.22)

	// Base ofensiva (preferir datos local/visitante de Footiqo si existen)
	homeTaken := home.AvgCornersFor * 1.05
	if home.HomeCorners != nil && home.HomeCorners.AvgCornersTaken > 0 {
		homeTaken = home.HomeCorners.AvgCornersTaken
	}
	awayTaken := away.AvgCornersFor * 0.95
	if away.AwayCorners != nil && away.AwayCorners.AvgCornersTaken > 0 {
		awayTaken = away.AwayCorners.AvgCornersTaken
	}

	// Base defensiva (corners concedidos por el rival, si existen)
	awayConceded := away.AvgCornersAgainst * 0.95
	if away.AwayCorners != nil && away.AwayCorners.AvgCornersConceded > 0 {
		awayConceded = away.AwayCorners.AvgCornersConceded
	}
	homeConceded := home.AvgCornersAgainst * 1.05
	if home.HomeCorners != nil && home.HomeCorners.AvgCornersConceded > 0 {
		homeConceded = home.HomeCorners.AvgCornersConceded
	}

	// Cruce equilibrado 50/50 entre ataque propio y concesión del rival
	homeExpected := homeTaken
	if awayConceded > 0 {
		if homeTaken > 0 {
			homeExpected = 0.5*homeTaken + 0.5*awayConceded
		} else {
			homeExpected = awayConceded
		}
	}

	awayExpected := awayTaken
	if homeConceded > 0 {
		if awayTaken > 0 {
			awayExpected = 0.5*awayTaken + 0.5*homeConceded
		} else {
			awayExpected = homeConceded
		}
	}

	homeExpected *= homeStyleFactor * strengthFactor
	awayExpected *= awayStyleFactor * (1.0 / strengthFactor)

	if homeExpected < 0 {
		homeExpected = 0
	}
	if awayExpected < 0 {
		awayExpected = 0
	}

	totalExpected := homeExpected + awayExpected

	// Varianzas escaladas manteniendo el índice de dispersión (Fano = Var / Media)
	calcCornerVar := func(t *Team, expected float64) float64 {
		if expected <= 0 {
			return 0
		}
		if t.CornerVariance > 0 {
			if t.AvgCornersFor > 0 {
				fano := clampFloat(t.CornerVariance/t.AvgCornersFor, 1.05, 4.0)
				return expected * fano
			}
			return t.CornerVariance
		}
		return expected * 1.75
	}

	homeVar := calcCornerVar(home, homeExpected)
	awayVar := calcCornerVar(away, awayExpected)
	totalVariance := homeVar + awayVar
	if totalVariance < 0 {
		totalVariance = 0
	}

	dist := make(map[int]float64)
	var over75, over85, over95, over105 float64

	if totalExpected > 0 {
		// Horizonte amplio para evitar truncamiento de cola derecha
		maxCorners := int(math.Ceil(totalExpected + 6.0*math.Sqrt(math.Max(totalVariance, totalExpected))))
		if maxCorners < 40 {
			maxCorners = 40
		}
		if maxCorners > 80 {
			maxCorners = 80
		}

		usePoisson := totalVariance <= totalExpected
		var r, pNb float64
		if !usePoisson {
			r = (totalExpected * totalExpected) / (totalVariance - totalExpected)
			if r > 200 {
				usePoisson = true
			} else {
				pNb = r / (r + totalExpected)
			}
		}

		probs := make([]float64, maxCorners+1)
		var massSum float64
		for total := 0; total <= maxCorners; total++ {
			var prob float64
			if usePoisson {
				prob = p.poisson(float64(total), totalExpected)
			} else {
				prob = p.negativeBinomial(float64(total), r, pNb)
			}
			probs[total] = prob
			massSum += prob
		}

		// Calcular CDF exacta sobre PMF normalizada para no perder masa en OverX
		if massSum > 0 {
			var cdf7, cdf8, cdf9, cdf10 float64
			for total := 0; total <= maxCorners; total++ {
				normProb := probs[total] / massSum
				if normProb > 0.0001 {
					dist[total] = normProb
				}
				if total <= 7 {
					cdf7 += normProb
				}
				if total <= 8 {
					cdf8 += normProb
				}
				if total <= 9 {
					cdf9 += normProb
				}
				if total <= 10 {
					cdf10 += normProb
				}
			}
			over75 = clampFloat(1.0-cdf7, 0, 1)
			over85 = clampFloat(1.0-cdf8, 0, 1)
			over95 = clampFloat(1.0-cdf9, 0, 1)
			over105 = clampFloat(1.0-cdf10, 0, 1)
		}
	} else {
		dist[0] = 1.0
	}

	stdDev := math.Sqrt(totalVariance)

	return &CornerPrediction{
		HomeExpected:  homeExpected,
		AwayExpected:  awayExpected,
		TotalExpected: totalExpected,
		TotalVariance: totalVariance,
		StdDev:        stdDev,
		Distribution:  dist,
		Over75:        over75,
		Over85:        over85,
		Over95:        over95,
		Over105:       over105,
	}
}

// negativeBinomial calcula probabilidad para distribucion binomial negativa usando Lgamma
func (p *Predictor) negativeBinomial(k, r, pVal float64) float64 {
	if k < 0 || r <= 0 || pVal <= 0 || pVal >= 1 {
		return 0
	}
	lg1, _ := math.Lgamma(k + r)
	lg2, _ := math.Lgamma(r)
	lg3, _ := math.Lgamma(k + 1)
	logProb := (lg1 - lg2 - lg3) + r*math.Log(pVal) + k*math.Log(1-pVal)
	val := math.Exp(logProb)
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0
	}
	return val
}

func effectiveBTTSRate(t *Team) (float64, bool) {
	if t == nil {
		return 0, false
	}
	if t.BTTSOverall > 0 {
		return clampFloat(t.BTTSOverall, 0, 1), true
	}
	if t.BTTSRate > 0 {
		return clampFloat(t.BTTSRate, 0, 1), true
	}
	// Si solo existen 1H y 2H (como en tables.yaml), estimar partido completo por unión
	if t.BTTS1H > 0 || t.BTTS2H > 0 {
		b1 := clampFloat(t.BTTS1H, 0, 1)
		b2 := clampFloat(t.BTTS2H, 0, 1)
		est := 1.0 - (1.0-b1)*(1.0-b2)
		return clampFloat(est, 0, 1), true
	}
	return 0, false
}

func (p *Predictor) CalculateBTTSProbability(home, away *Team) *BTTSPrediction {
	if home == nil || away == nil {
		return &BTTSPrediction{Probability: 0.5, YesOdds: 2.0, NoOdds: 2.0}
	}

	// 1. Goles esperados consistentes con PredictOutcome
	homeExpected, awayExpected := p.getExpectedGoals(home, away)

	// 2. Probabilidades marginales de anotar >= 1 gol bajo Poisson
	pHome1plus := 1.0 - math.Exp(-homeExpected)
	pAway1plus := 1.0 - math.Exp(-awayExpected)

	// 3. Correlación de liga/competición y probabilidad conjunta respetando cotas de Fréchet-Hoeffding
	isChampions := isChampionsCompetition(home.League, home.Competition) &&
		isChampionsCompetition(away.League, away.Competition)

	correlation := p.getCorrelation(home.League, home.Competition)
	if isChampions {
		correlation = p.getChampionsCorrelation()
	}
	correlation = clampFloat(correlation, -0.99, 0.99)

	lowerBound := math.Max(0.0, pHome1plus+pAway1plus-1.0)
	upperBound := math.Min(pHome1plus, pAway1plus)
	cov := correlation * math.Sqrt(pHome1plus*(1.0-pHome1plus)*pAway1plus*(1.0-pAway1plus))
	p11 := clampFloat(pHome1plus*pAway1plus+cov, lowerBound, upperBound)

	// 4. Combinación dinámica según datos disponibles (evitando ceros espurios)
	homeRef, okHome := effectiveBTTSRate(home)
	awayRef, okAway := effectiveBTTSRate(away)

	var histBTTS float64
	hasHist := false
	switch {
	case okHome && okAway:
		histBTTS = (homeRef + awayRef) / 2.0
		hasHist = true
	case okHome:
		histBTTS = homeRef
		hasHist = true
	case okAway:
		histBTTS = awayRef
		hasHist = true
	}

	wdlBTTS, hasWDL := p.calculateBTTSFromWDLWithFlag(home, away)

	wPoisson := 0.45
	wHist := 0.0
	wWDL := 0.0
	if hasHist {
		wHist = 0.30
	}
	if hasWDL {
		wWDL = 0.25
	}
	totalWeight := wPoisson + wHist + wWDL
	bttsProb := (wPoisson*p11 + wHist*histBTTS + wWDL*wdlBTTS) / totalWeight

	bttsProb = clampFloat(bttsProb, 0.001, 0.999)

	return &BTTSPrediction{
		Probability: bttsProb,
		YesOdds:     1.0 / bttsProb,
		NoOdds:      1.0 / (1.0 - bttsProb),
	}
}

// PredictOutcome devuelve la predicción del resultado del partido
func (p *Predictor) PredictOutcome(home, away *Team) *OutcomePrediction {
	if home == nil || away == nil {
		return &OutcomePrediction{
			HomeWinProb: 0.333, DrawProb: 0.334, AwayWinProb: 0.333,
			HomeOdds: 3.0, DrawOdds: 3.0, AwayOdds: 3.0,
		}
	}

	homeExpected, awayExpected := p.getExpectedGoals(home, away)

	maxGoals := 15
	homeProbs := make([]float64, maxGoals+1)
	awayProbs := make([]float64, maxGoals+1)

	for g := 0; g <= maxGoals; g++ {
		homeProbs[g] = p.poisson(float64(g), homeExpected)
		awayProbs[g] = p.poisson(float64(g), awayExpected)
	}

	var homeWin, draw, awayWin float64
	for i := 0; i <= maxGoals; i++ {
		for j := 0; j <= maxGoals; j++ {
			prob := homeProbs[i] * awayProbs[j]
			switch {
			case i > j:
				homeWin += prob
			case i == j:
				draw += prob
			default:
				awayWin += prob
			}
		}
	}

	total := homeWin + draw + awayWin
	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		return &OutcomePrediction{
			HomeWinProb: 0.333, DrawProb: 0.334, AwayWinProb: 0.333,
			HomeOdds: 3.0, DrawOdds: 3.0, AwayOdds: 3.0,
		}
	}

	homeWin = clampFloat(homeWin/total, 0.001, 0.999)
	draw = clampFloat(draw/total, 0.001, 0.999)
	awayWin = clampFloat(awayWin/total, 0.001, 0.999)

	// Re-normalizar para garantizar que HomeWinProb + DrawProb + AwayWinProb == 1.0
	normSum := homeWin + draw + awayWin
	homeWin /= normSum
	draw /= normSum
	awayWin /= normSum

	return &OutcomePrediction{
		HomeWinProb: homeWin,
		DrawProb:    draw,
		AwayWinProb: awayWin,
		HomeOdds:    1.0 / homeWin,
		DrawOdds:    1.0 / draw,
		AwayOdds:    1.0 / awayWin,
	}
}

// getExpectedGoals calcula los goles esperados para local y visitante
func (p *Predictor) getExpectedGoals(home, away *Team) (float64, float64) {
	homeXGFor, homeXGAgainst, homeFinDelta, _ := p.getXGForMatch(home, true)
	awayXGFor, awayXGAgainst, awayFinDelta, _ := p.getXGForMatch(away, false)

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

	isChampions := isChampionsCompetition(home.League, home.Competition) &&
		isChampionsCompetition(away.League, away.Competition)

	homeLocalFactor := 1.1
	awayAwayFactor := 0.95
	if isChampions {
		homeLocalFactor = 1.05
		awayAwayFactor = 0.98
		homeXGFor *= 1.08
		awayXGFor *= 1.08
	}

	avgXGHomeLeague := p.getAvgXG(home.League, home.Competition)
	avgXGAwayLeague := p.getAvgXG(away.League, away.Competition)
	avgXG := (avgXGHomeLeague + avgXGAwayLeague) / 2.0
	if isChampions {
		avgXG = 1.50
	}
	if avgXG <= 0 {
		avgXG = 1.35
	}

	homeDefQuality := homeXGAgainst / avgXG
	awayDefQuality := awayXGAgainst / avgXG
	if homeDefQuality <= 0 {
		homeDefQuality = 1.0
	}
	if awayDefQuality <= 0 {
		awayDefQuality = 1.0
	}

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

	strengthFactor := 1.0 + (home.PowerRating-away.PowerRating)/200.0
	if strengthFactor < 0.25 {
		strengthFactor = 0.25
	}

	homeExpected := homeXGFor * homeOffEfficiency * strengthFactor * homeLocalFactor * awayDefQuality
	awayExpected := awayXGFor * awayOffEfficiency * (1.0 / strengthFactor) * awayAwayFactor * homeDefQuality

	if homeExpected < 0 {
		homeExpected = 0
	}
	if awayExpected < 0 {
		awayExpected = 0
	}

	return homeExpected, awayExpected
}

// poisson calcula P(X = k) para una distribución Poisson con media lambda
func (p *Predictor) poisson(k, lambda float64) float64 {
	if lambda <= 0 {
		if k == 0 {
			return 1.0
		}
		return 0.0
	}
	if k < 0 {
		return 0.0
	}
	if k == 0 {
		return math.Exp(-lambda)
	}
	lg, _ := math.Lgamma(k + 1)
	val := math.Exp(-lambda + k*math.Log(lambda) - lg)
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0.0
	}
	return val
}

func (p *Predictor) calculateBTTSFromWDLWithFlag(home, away *Team) (float64, bool) {
	homeSum := home.WinHome + home.DrawHome + home.LossHome
	awaySum := away.WinAway + away.DrawAway + away.LossAway
	if homeSum <= 0 && awaySum <= 0 {
		return 0.5, false
	}

	pHomeScores := 0.5
	pAwayScores := 0.5

	// P(equipo anota) ≈ Win + 0.65*Draw + 0.35*Loss; P(rival concede) ≈ Loss + 0.65*Draw + 0.35*Win
	if homeSum > 0 && awaySum > 0 {
		homeAttack := home.WinHome + 0.65*home.DrawHome + 0.35*home.LossHome
		awayConcede := away.LossAway + 0.65*away.DrawAway + 0.35*away.WinAway
		pHomeScores = 0.5 * (homeAttack + awayConcede)

		awayAttack := away.WinAway + 0.65*away.DrawAway + 0.35*away.LossAway
		homeConcede := home.LossHome + 0.65*home.DrawHome + 0.35*home.WinHome
		pAwayScores = 0.5 * (awayAttack + homeConcede)
	} else if homeSum > 0 {
		pHomeScores = home.WinHome + 0.65*home.DrawHome + 0.35*home.LossHome
		pAwayScores = home.LossHome + 0.65*home.DrawHome + 0.35*home.WinHome
	} else {
		pAwayScores = away.WinAway + 0.65*away.DrawAway + 0.35*away.LossAway
		pHomeScores = away.LossAway + 0.65*away.DrawAway + 0.35*away.WinAway
	}

	pHomeScores = clampFloat(pHomeScores, 0.05, 0.95)
	pAwayScores = clampFloat(pAwayScores, 0.05, 0.95)
	return clampFloat(pHomeScores*pAwayScores, 0.1, 0.9), true
}

func (p *Predictor) calculateBTTSFromWDL(home, away *Team) float64 {
	est, _ := p.calculateBTTSFromWDLWithFlag(home, away)
	return est
}

// getCorrelation devuelve la correlación según liga y competición
func (p *Predictor) getCorrelation(league string, competition string) float64 {
	if isChampionsCompetition(league, competition) {
		return p.getChampionsCorrelation()
	}
	return p.getLeagueCorrelation(league)
}

func (p *Predictor) getChampionsCorrelation() float64 {
	return 0.22
}

func (p *Predictor) getLeagueCorrelation(league string) float64 {
	norm := normalizeLeagueName(league)
	leagueCorr := map[string]float64{
		"Premier League":          0.18,
		"Bundesliga":              0.22,
		"LaLiga":                  0.12,
		"Serie A":                 0.14,
		"Ligue 1":                 0.15,
		"Brazil Brazilian League": 0.15,
		"Liga MX":                 0.17,
		"Champions League":        0.22,
	}
	if val, ok := leagueCorr[norm]; ok {
		return val
	}
	return 0.15
}

// getAvgXG devuelve el promedio de xG según liga/competición
func (p *Predictor) getAvgXG(league string, competition string) float64 {
	if isChampionsCompetition(league, competition) {
		return 1.50
	}
	return p.getLeagueAvgXG(league)
}

func (p *Predictor) getLeagueAvgXG(league string) float64 {
	norm := normalizeLeagueName(league)
	leagueAvg := map[string]float64{
		"LaLiga":                  1.35,
		"Premier League":          1.45,
		"Bundesliga":              1.40,
		"Serie A":                 1.30,
		"Ligue 1":                 1.25,
		"Champions League":        1.50,
		"Brazil Brazilian League": 1.30,
		"Liga MX":                 1.38,
	}
	if val, ok := leagueAvg[norm]; ok {
		return val
	}
	return 1.35
}

// CalculateHandicap calcula probabilidades para handicap asiático del local (ej. -1.5, +0.5, -0.25)
func (p *Predictor) CalculateHandicap(home, away *Team, handicap float64) *HandicapPrediction {
	if home == nil || away == nil {
		return &HandicapPrediction{Handicap: handicap, HomeProb: 0.5, AwayProb: 0.5}
	}

	homeExpected, awayExpected := p.getExpectedGoals(home, away)
	expectedDiff := homeExpected - awayExpected

	if homeExpected == 0 && awayExpected == 0 {
		return &HandicapPrediction{
			Handicap:      handicap,
			HomeProb:      0.5,
			AwayProb:      0.5,
			ExpectedValue: 0,
		}
	}

	// Rejilla discreta doble-Poisson (Skellam) para soportar hándicaps enteros, medios y cuartos (.25/.75)
	maxGoals := 15
	homeProbs := make([]float64, maxGoals+1)
	awayProbs := make([]float64, maxGoals+1)
	for g := 0; g <= maxGoals; g++ {
		homeProbs[g] = p.poisson(float64(g), homeExpected)
		awayProbs[g] = p.poisson(float64(g), awayExpected)
	}

	var homeCover, awayCover float64
	for i := 0; i <= maxGoals; i++ {
		for j := 0; j <= maxGoals; j++ {
			prob := homeProbs[i] * awayProbs[j]
			margin := float64(i-j) + handicap
			switch {
			case margin >= 0.5-1e-9:
				homeCover += prob
			case margin > 1e-9:
				// Cuarto de gol a favor (+0.25): mitad gana, mitad reembolso
				homeCover += 0.5 * prob
			case margin <= -0.5+1e-9:
				awayCover += prob
			case margin < -1e-9:
				// Cuarto de gol en contra (-0.25): mitad pierde, mitad reembolso
				awayCover += 0.5 * prob
			}
		}
	}

	totalNoPush := homeCover + awayCover
	if totalNoPush <= 0 || math.IsNaN(totalNoPush) || math.IsInf(totalNoPush, 0) {
		return &HandicapPrediction{
			Handicap:      handicap,
			HomeProb:      0.5,
			AwayProb:      0.5,
			ExpectedValue: expectedDiff,
		}
	}

	homeProb := clampFloat(homeCover/totalNoPush, 0.001, 0.999)
	awayProb := 1.0 - homeProb

	return &HandicapPrediction{
		Handicap:      handicap,
		HomeProb:      homeProb,
		AwayProb:      awayProb,
		ExpectedValue: expectedDiff,
	}
}

// KellyCriterion calcula el criterio de Kelly para una apuesta
func (p *Predictor) KellyCriterion(probability, decimalOdds float64) float64 {
	if decimalOdds <= 1.0 || probability <= 0 || probability >= 1 {
		return 0
	}
	b := decimalOdds - 1.0
	q := 1.0 - probability

	kelly := (b*probability - q) / b
	if math.IsNaN(kelly) || math.IsInf(kelly, 0) || kelly < 0 {
		return 0
	}
	if kelly > 0.25 {
		return 0.25
	}
	return kelly
}

// Edge calcula el valor esperado porcentual (EV%) respecto a la cuota decimal
func (p *Predictor) Edge(probability, decimalOdds float64) float64 {
	if probability <= 0 || decimalOdds <= 0 {
		return 0
	}
	edge := (probability*decimalOdds - 1.0) * 100.0
	if math.IsNaN(edge) || math.IsInf(edge, 0) {
		return 0
	}
	return edge
}

// RecommendBet recomienda si apostar basado en Kelly y Edge
func (p *Predictor) RecommendBet(probability, decimalOdds float64, minKelly float64) (bool, float64, float64) {
	kelly := p.KellyCriterion(probability, decimalOdds)
	edge := p.Edge(probability, decimalOdds)

	return kelly >= minKelly, kelly, edge
}

// LoadBTTSData carga porcentajes BTTS desde CSV (Team, Matches Played, BTTS YES %, BTTS NO %)
func (p *Predictor) LoadBTTSData(bttsFile string, period string) error {
	file, err := os.Open(bttsFile)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}
	if len(records) <= 1 {
		return nil
	}

	headers := mapHeaders(records[0])
	headerCount := len(records[0])
	normPeriod := strings.ToUpper(strings.TrimSpace(period))

	for i, record := range records {
		if i == 0 || isEmptyRecord(record) {
			continue
		}

		teamIdx := 0
		bttsIdx := 2
		if len(record) >= 7 {
			teamIdx = 3
			bttsIdx = 5
		}
		if len(record) == headerCount {
			if idx, ok := lookupHeader(headers, "team", "equipo"); ok {
				teamIdx = idx
			}
			if idx, ok := lookupHeader(headers, "btts yes %", "btts yes", "btts yes (%)"); ok {
				bttsIdx = idx
			}
		}

		if len(record) <= teamIdx || len(record) <= bttsIdx {
			continue
		}

		teamName := strings.TrimSpace(record[teamIdx])
		if teamName == "" {
			continue
		}

		prop, err := parsePercentProportion(record[bttsIdx])
		if err != nil {
			continue
		}

		team := p.findTeamByName(teamName)
		if team == nil {
			continue
		}

		switch normPeriod {
		case "1H":
			team.BTTS1H = prop
		case "2H":
			team.BTTS2H = prop
		case "FT", "OVERALL", "":
			team.BTTSOverall = prop
		}
	}
	return nil
}

// LoadXGSeparatedData carga xG separado por local/visitante
func (p *Predictor) LoadXGSeparatedData(homeFile, awayFile string) error {
	if err := p.loadXGFile(homeFile, true); err != nil {
		return err
	}
	return p.loadXGFile(awayFile, false)
}

func (p *Predictor) loadXGFile(filename string, isHome bool) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}
	if len(records) <= 1 {
		return nil
	}

	headers := mapHeaders(records[0])
	headerCount := len(records[0])

	hasCurrentSeason := false
	for i, record := range records {
		if i == 0 || isEmptyRecord(record) || len(record) == 0 {
			continue
		}
		pIdx := 0
		if len(record) == headerCount {
			if idx, ok := lookupHeader(headers, "period", "timeframe"); ok {
				pIdx = idx
			} else {
				pIdx = -1
			}
		} else if len(record) < 11 {
			pIdx = -1
		}
		if pIdx >= 0 && pIdx < len(record) && strings.EqualFold(strings.TrimSpace(record[pIdx]), "Current season") {
			hasCurrentSeason = true
			break
		}
	}

	fileIsChampions := isChampionsCompetition(filename, "")

	for i, record := range records {
		if i == 0 || isEmptyRecord(record) {
			continue
		}

		periodIdx := -1
		leagueIdx := -1
		var teamIdx, xgForIdx, xgAgainstIdx, finDeltaIdx, concDeltaIdx int

		switch {
		case len(record) == headerCount && len(record) >= 11:
			if idx, ok := lookupHeader(headers, "period", "timeframe"); ok {
				periodIdx = idx
			}
			if idx, ok := lookupHeader(headers, "league"); ok {
				leagueIdx = idx
			}
			if idx, ok := lookupHeader(headers, "team", "equipo"); ok {
				teamIdx = idx
			} else {
				teamIdx = 3
			}
			if idx, ok := lookupHeader(headers, "avg xg for", "xg for"); ok {
				xgForIdx = idx
			} else {
				xgForIdx = 5
			}
			if idx, ok := lookupHeader(headers, "avg xg against", "xg against"); ok {
				xgAgainstIdx = idx
			} else {
				xgAgainstIdx = 6
			}
			if idx, ok := lookupHeader(headers, "finishing delta"); ok {
				finDeltaIdx = idx
			} else if len(record) == 11 {
				finDeltaIdx = 9
			} else {
				finDeltaIdx = 10
			}
			if idx, ok := lookupHeader(headers, "conceded delta"); ok {
				concDeltaIdx = idx
			} else if len(record) == 11 {
				concDeltaIdx = 10
			} else {
				concDeltaIdx = 11
			}
		case len(record) >= 12:
			periodIdx = 0
			leagueIdx = 2
			teamIdx = 3
			xgForIdx = 5
			xgAgainstIdx = 6
			finDeltaIdx = 10
			concDeltaIdx = 11
		case len(record) == 11:
			periodIdx = 0
			leagueIdx = 2
			teamIdx = 3
			xgForIdx = 5
			xgAgainstIdx = 6
			finDeltaIdx = 9
			concDeltaIdx = 10
		case len(record) >= 8:
			teamIdx = 0
			xgForIdx = 2
			xgAgainstIdx = 3
			finDeltaIdx = 6
			concDeltaIdx = 7
		default:
			continue
		}

		if len(record) <= teamIdx || len(record) <= xgForIdx || len(record) <= xgAgainstIdx ||
			len(record) <= finDeltaIdx || len(record) <= concDeltaIdx {
			continue
		}

		if hasCurrentSeason && periodIdx >= 0 && periodIdx < len(record) {
			periodVal := strings.TrimSpace(record[periodIdx])
			if periodVal != "" && !strings.EqualFold(periodVal, "Current season") {
				continue
			}
		}

		teamName := strings.TrimSpace(record[teamIdx])
		if teamName == "" {
			continue
		}

		team := p.findTeamByName(teamName)
		if team == nil {
			continue
		}

		// No contaminar equipos de ligas domésticas al cargar xG de Champions League
		leagueVal := ""
		if leagueIdx >= 0 && leagueIdx < len(record) {
			leagueVal = strings.TrimSpace(record[leagueIdx])
		}
		if team.Competition == "" && team.League == "" && (isChampionsCompetition(leagueVal, "") || fileIsChampions) {
			team.Competition = "Champions"
		}

		xgFor, errFor := parseCleanFloat(record[xgForIdx])
		xgAgainst, errAgainst := parseCleanFloat(record[xgAgainstIdx])
		finDelta, errFin := parseCleanFloat(record[finDeltaIdx])
		concDelta, errConc := parseCleanFloat(record[concDeltaIdx])

		if isHome {
			if errFor == nil {
				team.XGForHome = xgFor
			}
			if errAgainst == nil {
				team.XGAgainstHome = xgAgainst
			}
			if errFin == nil {
				team.FinishingDeltaHome = finDelta
			}
			if errConc == nil {
				team.ConcededDeltaHome = concDelta
			}
		} else {
			if errFor == nil {
				team.XGForAway = xgFor
			}
			if errAgainst == nil {
				team.XGAgainstAway = xgAgainst
			}
			if errFin == nil {
				team.FinishingDeltaAway = finDelta
			}
			if errConc == nil {
				team.ConcededDeltaAway = concDelta
			}
		}
	}
	return nil
}

func (p *Predictor) getXGForMatch(team *Team, isHome bool) (xgFor, xgAgainst, finishingDelta, concededDelta float64) {
	if team == nil {
		return 0, 0, 0, 0
	}
	if isHome {
		return team.XGForHome, team.XGAgainstHome, team.FinishingDeltaHome, team.ConcededDeltaHome
	}
	return team.XGForAway, team.XGAgainstAway, team.FinishingDeltaAway, team.ConcededDeltaAway
}

// LoadWDLData carga porcentajes de Win/Draw/Loss desde CSV
func (p *Predictor) LoadWDLData(wdlFile string, location string) error {
	file, err := os.Open(wdlFile)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}
	if len(records) <= 1 {
		return nil
	}

	headers := mapHeaders(records[0])
	headerCount := len(records[0])
	normLoc := strings.ToLower(strings.TrimSpace(location))

	for i, record := range records {
		if i == 0 || isEmptyRecord(record) {
			continue
		}

		var teamIdx, winIdx, drawIdx, lossIdx int
		switch {
		case len(record) == headerCount && len(record) >= 5:
			if idx, ok := lookupHeader(headers, "team", "equipo"); ok {
				teamIdx = idx
			} else if len(record) >= 8 {
				teamIdx = 3
			} else {
				teamIdx = 0
			}
			if idx, ok := lookupHeader(headers, "win %", "win", "win (%)"); ok {
				winIdx = idx
			} else if len(record) >= 8 {
				winIdx = 5
			} else {
				winIdx = 2
			}
			if idx, ok := lookupHeader(headers, "draw %", "draw", "draw (%)"); ok {
				drawIdx = idx
			} else if len(record) >= 8 {
				drawIdx = 6
			} else {
				drawIdx = 3
			}
			if idx, ok := lookupHeader(headers, "loss %", "loss", "loss (%)"); ok {
				lossIdx = idx
			} else if len(record) >= 8 {
				lossIdx = 7
			} else {
				lossIdx = 4
			}
		case len(record) >= 8:
			teamIdx = 3
			winIdx = 5
			drawIdx = 6
			lossIdx = 7
		case len(record) >= 5:
			teamIdx = 0
			winIdx = 2
			drawIdx = 3
			lossIdx = 4
		default:
			continue
		}

		if len(record) <= teamIdx || len(record) <= winIdx || len(record) <= drawIdx || len(record) <= lossIdx {
			continue
		}

		teamName := strings.TrimSpace(record[teamIdx])
		if teamName == "" {
			continue
		}

		team := p.findTeamByName(teamName)
		if team == nil {
			continue
		}

		winProp, errWin := parsePercentProportion(record[winIdx])
		drawProp, errDraw := parsePercentProportion(record[drawIdx])
		lossProp, errLoss := parsePercentProportion(record[lossIdx])

		switch normLoc {
		case "home":
			if errWin == nil {
				team.WinHome = winProp
			}
			if errDraw == nil {
				team.DrawHome = drawProp
			}
			if errLoss == nil {
				team.LossHome = lossProp
			}
		case "away":
			if errWin == nil {
				team.WinAway = winProp
			}
			if errDraw == nil {
				team.DrawAway = drawProp
			}
			if errLoss == nil {
				team.LossAway = lossProp
			}
		}
	}

	return nil
}

// LoadCornersData carga estadísticas de corners desde CSV
func (p *Predictor) LoadCornersData(filename string, location string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}
	if len(records) <= 1 {
		return nil
	}

	headers := mapHeaders(records[0])
	headerCount := len(records[0])
	normLoc := strings.ToLower(strings.TrimSpace(location))

	thresholds := []float64{7.5, 8.5, 9.5, 10.5}
	overKeys := [][]string{
		{"over 7.5 (%)", "over 7.5 %", "over 7.5"},
		{"over 8.5 (%)", "over 8.5 %", "over 8.5"},
		{"over 9.5 (%)", "over 9.5 %", "over 9.5"},
		{"over 10.5 (%)", "over 10.5 %", "over 10.5"},
	}
	underKeys := [][]string{
		{"under 7.5 (%)", "under 7.5 %", "under 7.5"},
		{"under 8.5 (%)", "under 8.5 %", "under 8.5"},
		{"under 9.5 (%)", "under 9.5 %", "under 9.5"},
		{"under 10.5 (%)", "under 10.5 %", "under 10.5"},
	}

	for i, record := range records {
		if i == 0 || isEmptyRecord(record) {
			continue
		}
		if len(record) < 11 {
			continue
		}

		offset := 0
		if len(record) >= 15 {
			offset = 3
		}

		teamIdx := 0 + offset
		matchesIdx := 1 + offset
		takenIdx := 2 + offset
		concededIdx := 3 + offset
		aligned := len(record) == headerCount

		if aligned {
			if idx, ok := lookupHeader(headers, "team", "equipo"); ok {
				teamIdx = idx
			}
			if idx, ok := lookupHeader(headers, "matches played", "matches"); ok {
				matchesIdx = idx
			}
			if idx, ok := lookupHeader(headers, "avg corners taken", "corners taken"); ok {
				takenIdx = idx
			}
			if idx, ok := lookupHeader(headers, "avg corners conceded", "corners conceded"); ok {
				concededIdx = idx
			}
		}

		if len(record) <= teamIdx || len(record) <= takenIdx || len(record) <= concededIdx {
			continue
		}

		teamName := strings.TrimSpace(record[teamIdx])
		if teamName == "" {
			continue
		}

		team := p.findTeamByName(teamName)
		if team == nil {
			continue
		}

		var stats *CornerStats
		if normLoc == "home" && team.HomeCorners != nil {
			stats = team.HomeCorners
		} else if normLoc != "home" && team.AwayCorners != nil {
			stats = team.AwayCorners
		} else {
			stats = &CornerStats{
				OverPercent:  make(map[float64]float64),
				UnderPercent: make(map[float64]float64),
			}
		}

		if matchesIdx < len(record) {
			if matches, err := parseCleanInt(record[matchesIdx]); err == nil && matches >= 0 {
				stats.MatchesPlayed = matches
			}
		}

		hasValidMetric := false
		if avgTaken, err := parseCleanFloat(record[takenIdx]); err == nil && avgTaken >= 0 {
			stats.AvgCornersTaken = avgTaken
			hasValidMetric = true
		}
		if avgConceded, err := parseCleanFloat(record[concededIdx]); err == nil && avgConceded >= 0 {
			stats.AvgCornersConceded = avgConceded
			hasValidMetric = true
		}

		for j, th := range thresholds {
			overIdx := 4 + offset + j*2
			underIdx := 5 + offset + j*2
			if aligned {
				if idx, ok := lookupHeader(headers, overKeys[j]...); ok {
					overIdx = idx
				}
				if idx, ok := lookupHeader(headers, underKeys[j]...); ok {
					underIdx = idx
				}
			}

			if overIdx >= 0 && overIdx < len(record) {
				if overProp, err := parsePercentProportion(record[overIdx]); err == nil {
					stats.OverPercent[th] = overProp
					hasValidMetric = true
				}
			}
			if underIdx >= 0 && underIdx < len(record) {
				if underProp, err := parsePercentProportion(record[underIdx]); err == nil {
					stats.UnderPercent[th] = underProp
					hasValidMetric = true
				}
			}
		}

		if !hasValidMetric {
			continue
		}

		if normLoc == "home" {
			team.HomeCorners = stats
		} else {
			team.AwayCorners = stats
		}
	}
	return nil
}

func (p *Predictor) findTeamByName(name string) *Team {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" || len(p.Teams) == 0 {
		return nil
	}

	keys := make([]string, 0, len(p.Teams))
	for id := range p.Teams {
		keys = append(keys, id)
	}
	sort.Strings(keys)

	// 1. Coincidencia exacta (case-insensitive)
	for _, id := range keys {
		team := p.Teams[id]
		if strings.EqualFold(strings.TrimSpace(team.Name), cleanName) {
			return team
		}
	}

	// 2. Coincidencia normalizada (sin diacríticos, puntuación básica ni espacios extras)
	normQuery := normalizeTeamName(cleanName)
	if normQuery == "" {
		return nil
	}
	for _, id := range keys {
		team := p.Teams[id]
		if normalizeTeamName(team.Name) == normQuery {
			return team
		}
	}

	// 3. Coincidencia parcial segura: solo con longitud suficiente y sin ambigüedad
	if len(normQuery) < 5 {
		return nil
	}
	var candidates []*Team
	for _, id := range keys {
		team := p.Teams[id]
		normTeam := normalizeTeamName(team.Name)
		if len(normTeam) < 5 {
			continue
		}
		if strings.Contains(normTeam, normQuery) || strings.Contains(normQuery, normTeam) {
			candidates = append(candidates, team)
		}
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	return nil
}
