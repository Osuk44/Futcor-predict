package models

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
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
	BTTSRate          float64 // ← Lo mantenemos pero ya no lo usaremos
	GoalsForAvg       float64
	GoalsAgainstAvg   float64
	HandicapStats     map[float64]float64
	
	// xG separado por local/visitante (NUEVO)
    XGForHome      float64
    XGAgainstHome  float64
    XGForAway      float64
    XGAgainstAway  float64
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

// LoadTeamsFromCSV carga equipos desde archivo CSV
func (p *Predictor) LoadTeamsFromCSV(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	loadedCount := 0
	skippedCount := 0

	for i, record := range records {
		if i == 0 {
			continue // Saltar cabecera
		}

		// Validar que tenemos al menos 13 columnas (ahora incluimos varianza opcional)
		if len(record) < 12 { // Mínimo necesario sin varianza
			fmt.Printf("⚠️ Fila %d saltada: columnas insuficientes (%d)\n", i+1, len(record))
			skippedCount++
			continue
		}

		// Función auxiliar para parsear floats
		parseFloat := func(s string) (float64, error) {
			return strconv.ParseFloat(strings.TrimSpace(s), 64)
		}

		// Parsear campos obligatorios
		id := strings.TrimSpace(record[0])
		name := strings.TrimSpace(record[1])
		league := strings.TrimSpace(record[2])

		powerRating, err := parseFloat(record[3])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: PowerRating inválido (%s)\n", i+1, record[3])
			skippedCount++
			continue
		}

		gax, err := parseFloat(record[4])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: GAx inválido (%s)\n", i+1, record[4])
			skippedCount++
			continue
		}

		gsax, err := parseFloat(record[5])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: GSAx inválido (%s)\n", i+1, record[5])
			skippedCount++
			continue
		}

		style := strings.ToUpper(strings.TrimSpace(record[6]))

		avgCornersFor, err := parseFloat(record[7])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: AvgCornersFor inválido (%s)\n", i+1, record[7])
			skippedCount++
			continue
		}

		avgCornersAgainst, err := parseFloat(record[8])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: AvgCornersAgainst inválido (%s)\n", i+1, record[8])
			skippedCount++
			continue
		}

		bttsRate, err := parseFloat(record[9])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: BTTSRate inválido (%s)\n", i+1, record[9])
			skippedCount++
			continue
		}

		goalsForAvg, err := parseFloat(record[10])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: GoalsForAvg inválido (%s)\n", i+1, record[10])
			skippedCount++
			continue
		}

		goalsAgainstAvg, err := parseFloat(record[11])
		if err != nil {
			fmt.Printf("⚠️ Fila %d saltada: GoalsAgainstAvg inválido (%s)\n", i+1, record[11])
			skippedCount++
			continue
		}

		// Varianza de corners (columna opcional, índice 12)
		var cornerVariance float64
		if len(record) > 12 {
			cornerVariance, _ = parseFloat(record[12]) // Si falla, se queda 0
		}
		if cornerVariance == 0 {
			// Estimación por defecto: varianza = media * 1.5 (razonable)
			cornerVariance = avgCornersFor * 1.5
		}

		// Crear equipo
		team := &Team{
			ID:                id,
			Name:              name,
			League:            league,
			PowerRating:       powerRating,
			GAx:               gax,
			GSAx:              gsax,
			Style:             style,
			AvgCornersFor:     avgCornersFor,
			AvgCornersAgainst: avgCornersAgainst,
			CornerVariance:    cornerVariance,
			BTTSRate:          bttsRate / 100,
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

// CalculateCornerExpectation calcula corners esperados ajustando por estilo y fortaleza
func (p *Predictor) CalculateCornerExpectation(home, away *Team) *CornerPrediction {
	// Validar varianzas (por si acaso)
	if home.CornerVariance <= 0 {
		home.CornerVariance = home.AvgCornersFor * 1.5
	}
	if away.CornerVariance <= 0 {
		away.CornerVariance = away.AvgCornersFor * 1.5
	}

	// Factor de estilo
	getStyleFactor := func(style string) float64 {
		factors := map[string]float64{
			"BANDA":    1.3,
			"MIXTO":    1.1,
			"CENTRO":   0.9,
			"WINGPLAY": 1.25,
		}
		if val, ok := factors[style]; ok {
			return val
		}
		return 1.0 // neutro
	}

	homeStyleFactor := getStyleFactor(home.Style)
	awayStyleFactor := getStyleFactor(away.Style)

	// Factor de fuerza relativa
	strengthFactor := 1.0 + (home.PowerRating-away.PowerRating)/100

	// Usar datos específicos de local/visitante si existen, sino usar promedios con factor localía
	homeExpected := home.AvgCornersFor
	if home.HomeCorners != nil {
		homeExpected = home.HomeCorners.AvgCornersTaken
	} else {
		homeExpected = home.AvgCornersFor * 1.1 // fallback localía
	}

	awayExpected := away.AvgCornersFor
	if away.AwayCorners != nil {
		awayExpected = away.AwayCorners.AvgCornersTaken
	} else {
		awayExpected = away.AvgCornersFor * 0.95 // fallback visitante
	}

	// Aplicar factores de estilo y fuerza (sin duplicar localía)
	homeExpected *= homeStyleFactor * strengthFactor
	awayExpected *= awayStyleFactor * (1 / strengthFactor)

	totalExpected := homeExpected + awayExpected
	totalVariance := home.CornerVariance + away.CornerVariance

	// Parámetros de binomial negativa
	var r, p_nb float64
	if totalVariance > totalExpected {
		r = (totalExpected * totalExpected) / (totalVariance - totalExpected)
		p_nb = r / (r + totalExpected)
	} else {
		// Varianza baja → aproximación Poisson (r grande)
		r = 20.0
		p_nb = r / (r + totalExpected)
	}

	// Generar distribución
	dist := make(map[int]float64)
	for total := 0; total <= 25; total++ {
		prob := p.negativeBinomial(float64(total), r, p_nb)
		if prob > 0.0001 {
			dist[total] = prob
		}
	}

	// Calcular probabilidades acumuladas
	var over75, over85, over95, over105 float64
	for total, prob := range dist {
		if total > 7 {
			over75 += prob
		}
		if total > 8 {
			over85 += prob
		}
		if total > 9 {
			over95 += prob
		}
		if total > 10 {
			over105 += prob
		}
	}

	// Calcular desviación estándar con la varianza ya calculada
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

// negativeBinomial calcula probabilidad para distribucion binomial negativa
func (p *Predictor) negativeBinomial(k, r, p_val float64) float64 {
	// P(X=k) = C(k+r-1, k) * p^r * (1-p)^k
	coef := math.Gamma(k+r) / (math.Gamma(r) * math.Gamma(k+1))
	return coef * math.Pow(p_val, r) * math.Pow(1-p_val, k)
}

func (p *Predictor) CalculateBTTSProbability(home, away *Team) *BTTSPrediction {
	// 1. Obtener xG específicos (SOLO local/visitante, sin fallback)
	homeXGFor, homeXGAgainst, homeFinDelta, homeConcDelta := p.getXGForMatch(home, true)
	awayXGFor, awayXGAgainst, awayFinDelta, awayConcDelta := p.getXGForMatch(away, false)
	
	// USAR las variables que estaban "no usadas"
	_ = homeConcDelta  // O úsalas en el cálculo de suerte más abajo
	_ = awayConcDelta

	// 2. Si no hay datos xG, usar estimación desde goles reales con factor de conversión
	if homeXGFor == 0 {
		homeXGFor = home.GoalsForAvg * 0.85  // xG suele ser ~85% de goles reales
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

	// 3. Detectar Champions
	isChampions := home.Competition == "Champions" && away.Competition == "Champions"
	
	// 4. Obtener correlación y promedio xG
	correlation := p.getCorrelation(home.League, home.Competition)
	avgXG := p.getAvgXG(home.League, home.Competition)
	
	// Calidad defensiva
	homeDefQuality := homeXGAgainst / avgXG
	awayDefQuality := awayXGAgainst / avgXG

	// 5. Factores de localía/visitante
	homeLocalFactor := 1.1
	awayAwayFactor := 0.95
	
	if isChampions {
		homeLocalFactor = 1.05
		awayAwayFactor = 0.98
		correlation = 0.22
		
		homeXGFor *= 1.08
		awayXGFor *= 1.08
	}

	// 6. Eficiencia ofensiva
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

	// 7. Goles esperados
	strengthFactor := 1.0 + (home.PowerRating-away.PowerRating)/200
	
	homeExpected := homeXGFor * homeOffEfficiency * strengthFactor * homeLocalFactor
	awayExpected := awayXGFor * awayOffEfficiency * (1/strengthFactor) * awayAwayFactor

	// Ajuste defensivo
	homeExpected *= awayDefQuality
	awayExpected *= homeDefQuality

	// 8. Probabilidades Poisson
	pHome0 := math.Exp(-homeExpected)
	pAway0 := math.Exp(-awayExpected)
	pHome1plus := 1 - pHome0
	pAway1plus := 1 - pAway0

	// 9. Matriz bivariada
	p11 := pHome1plus * pAway1plus * (1 + correlation)

	// 10. Factor de suerte (AHORA SÍ usamos homeConcDelta)
	luckFactor := 1.0
	if math.Abs(homeFinDelta+homeConcDelta) > 0.5 || math.Abs(awayFinDelta+awayConcDelta) > 0.5 {
		luckFactor = 0.98
	}

	// 11. Combinación final
	bttsProb := 0.45*p11 + 
				0.30*(home.BTTSOverall+away.BTTSOverall)/2 + 
				0.25*p.calculateBTTSFromWDL(home, away)
	bttsProb *= luckFactor

	if isChampions {
		bttsProb *= 1.03
	}

	return &BTTSPrediction{
		Probability: bttsProb,
		YesOdds:     1 / bttsProb,
		NoOdds:      1 / (1 - bttsProb),
	}
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

// PredictOutcome devuelve la predicción del resultado del partido
func (p *Predictor) PredictOutcome(home, away *Team) *OutcomePrediction {
	// 1. Obtener goles esperados
	homeExpected, awayExpected := p.getExpectedGoals(home, away)

	// 2. Calcular probabilidades Poisson para 0..10 goles
	maxGoals := 10
	homeProbs := make([]float64, maxGoals+1)
	awayProbs := make([]float64, maxGoals+1)

	for g := 0; g <= maxGoals; g++ {
		homeProbs[g] = p.poisson(float64(g), homeExpected)
		awayProbs[g] = p.poisson(float64(g), awayExpected)
	}

	// 3. Sumar probabilidades de marcadores
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

	// Normalizar por si acaso (debería sumar 1)
	total := homeWin + draw + awayWin
	homeWin /= total
	draw /= total
	awayWin /= total

	return &OutcomePrediction{
		HomeWinProb: homeWin,
		DrawProb:    draw,
		AwayWinProb: awayWin,
		HomeOdds:    1 / homeWin,
		DrawOdds:    1 / draw,
		AwayOdds:    1 / awayWin,
	}
}

// getExpectedGoals calcula los goles esperados para local y visitante
// basado en la misma lógica que CalculateBTTSProbability (sin el blending final)
func (p *Predictor) getExpectedGoals(home, away *Team) (float64, float64) {
	// Obtener xG específicos (local/visitante) con fallback a goles promedio
	homeXGFor, homeXGAgainst, homeFinDelta, _ := p.getXGForMatch(home, true)
	awayXGFor, awayXGAgainst, awayFinDelta, _ := p.getXGForMatch(away, false)

	// Si no hay xG, estimar desde goles reales (factor 0.85)
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

	// Detectar si es Champions (para ajustes específicos)
	isChampions := home.Competition == "Champions" && away.Competition == "Champions"

	// Factores de localía/visitante
	homeLocalFactor := 1.1
	awayAwayFactor := 0.95
	if isChampions {
		homeLocalFactor = 1.05
		awayAwayFactor = 0.98
		// Ajuste ofensivo extra en Champions
		homeXGFor *= 1.08
		awayXGFor *= 1.08
	}

	// Calidad defensiva respecto al promedio de la liga
	avgXG := p.getAvgXG(home.League, home.Competition)
	homeDefQuality := homeXGAgainst / avgXG
	awayDefQuality := awayXGAgainst / avgXG

	// Eficiencia ofensiva basada en finishing delta
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

	// Factor de fuerza relativa
	strengthFactor := 1.0 + (home.PowerRating-away.PowerRating)/200

	// Goles esperados finales
	homeExpected := homeXGFor * homeOffEfficiency * strengthFactor * homeLocalFactor
	awayExpected := awayXGFor * awayOffEfficiency * (1/strengthFactor) * awayAwayFactor

	// Ajuste defensivo mutuo
	homeExpected *= awayDefQuality
	awayExpected *= homeDefQuality

	return homeExpected, awayExpected
}
// poisson calcula P(X = k) para una distribución Poisson con media lambda
func (p *Predictor) poisson(k, lambda float64) float64 {
	if lambda == 0 {
		if k == 0 {
			return 1.0
		}
		return 0.0
	}
	return math.Exp(-lambda) * math.Pow(lambda, k) / math.Gamma(k+1)
}

func (p *Predictor) calculateBTTSFromWDL(home, away *Team) float64 {
	// Promedio de no-victorias
	homeNonWin := home.DrawHome + home.LossHome
	awayNonWin := away.DrawAway + away.LossAway
	
	bttsEstimate := (homeNonWin + awayNonWin) / 2
	
	if bttsEstimate < 0.4 {
		return 0.4
	}
	if bttsEstimate > 0.6 {
		return 0.6
	}
	return bttsEstimate
}

// getCorrelation devuelve la correlación según liga y competición
func (p *Predictor) getCorrelation(league string, competition string) float64 {
    // Primero verificar si es competición europea
    if competition == "Champions" {
        return p.getChampionsCorrelation()
    }
     // Si no, usar correlación por liga doméstica
    return p.getLeagueCorrelation(league)
}

// Función auxiliar para correlación de Champions
func (p *Predictor) getChampionsCorrelation() float64 {
    return 0.22 // Basado en datos históricos de Champions
}

// getLeagueCorrelation - para ligas domésticas
func (p *Predictor) getLeagueCorrelation(league string) float64 {
    leagueCorr := map[string]float64{
        "Premier League": 0.18,
        "Bundesliga":     0.22,
        "LaLiga":         0.12,
        "Serie A":        0.14,
        "Ligue 1":        0.15,
    }
    if val, ok := leagueCorr[league]; ok {
        return val
    }
    return 0.15 // valor por defecto
}

// getAvgXG devuelve el promedio de xG según liga/competición
func (p *Predictor) getAvgXG(league string, competition string) float64 {
    // Si es competición europea, usar promedio específico
    if competition == "Champions" {
        return 1.50
    }
   
    // Si no, usar promedio por liga
    return p.getLeagueAvgXG(league)
}

// Función auxiliar para obtener promedio de xG de la liga
func (p *Predictor) getLeagueAvgXG(league string) float64 {
    // Puedes calcularlo dinámicamente o tener valores predefinidos
    leagueAvg := map[string]float64{
        "LaLiga":          1.35,
        "Premier League":  1.45,
        "Bundesliga":      1.40,
        "Serie A":         1.30,
        "Ligue 1":         1.25,
        "Champions League":1.50,
    }
    if val, ok := leagueAvg[league]; ok {
        return val
    }
    return 1.35 // valor por defecto
}

// CalculateHandicap calcula probabilidades para handicap
func (p *Predictor) CalculateHandicap(home, away *Team, handicap float64) *HandicapPrediction {
	// Diferencia de goles esperada
	expectedDiff := (home.GoalsForAvg*1.1 - away.GoalsAgainstAvg*0.95) - (away.GoalsForAvg*0.95 - home.GoalsAgainstAvg*1.1)

	// Varianza estimada
	variance := (home.GoalsForAvg + away.GoalsAgainstAvg +
		away.GoalsForAvg + home.GoalsAgainstAvg) / 4

	// Probabilidad de cubrir handicap usando distribucion normal
	stdDev := math.Sqrt(variance)

	// Probabilidad de que local gane por mas de handicap
	zScore := (expectedDiff - handicap) / stdDev
	homeProb := 0.5 * (1 + math.Erf(zScore/math.Sqrt(2)))

	// Probabilidad de que visita cubra handicap (local pierde o gana por menos)
	zScoreAway := (expectedDiff - (-handicap)) / stdDev
	awayProb := 1 - 0.5*(1+math.Erf(zScoreAway/math.Sqrt(2)))

	return &HandicapPrediction{
		Handicap:      handicap,
		HomeProb:      homeProb,
		AwayProb:      awayProb,
		ExpectedValue: expectedDiff,
	}
}

// KellyCriterion calcula el criterio de Kelly para una apuesta
func (p *Predictor) KellyCriterion(probability, decimalOdds float64) float64 {
	// f* = (bp - q) / b
	// b = decimalOdds - 1
	b := decimalOdds - 1
	q := 1 - probability

	kelly := (b*probability - q) / b

	// Limitar entre 0 y 0.25 (gestion de riesgo)
	if kelly < 0 {
		return 0
	}
	if kelly > 0.25 {
		return 0.25
	}
	return kelly
}

// Edge calcula la ventaja respecto a la cuota
func (p *Predictor) Edge(probability, decimalOdds float64) float64 {
	fairOdds := 1 / probability
	return (fairOdds - decimalOdds) / decimalOdds * 100 // Porcentaje
}

// RecommendBet recomienda si apostar basado en Kelly y Edge
func (p *Predictor) RecommendBet(probability, decimalOdds float64, minKelly float64) (bool, float64, float64) {
	kelly := p.KellyCriterion(probability, decimalOdds)
	edge := p.Edge(probability, decimalOdds)

	return kelly >= minKelly, kelly, edge
}

func (p *Predictor) LoadBTTSData(bttsFile string, period string) error {
	file, err := os.Open(bttsFile)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	for i, record := range records {
		if i == 0 {
			continue
		}
		if len(record) < 3 {
			continue
		}

		teamName := record[0]
		bttsPercent := strings.TrimSuffix(record[1], "%") // La columna "BTTS YES %"
		value, err := strconv.ParseFloat(bttsPercent, 64)
		if err != nil {
			continue
		}
		value = value / 100

		team := p.findTeamByName(teamName)
		if team == nil {
			continue
		}

		switch period {
		case "1H":
			team.BTTS1H = value
		case "2H":
			team.BTTS2H = value
		case "FT":
			team.BTTSOverall = value
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
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	for i, record := range records {
		if i == 0 {
			continue
		}
		if len(record) < 8 {
			continue
		}

		teamName := record[0]
		xgFor, _ := strconv.ParseFloat(record[2], 64)
		xgAgainst, _ := strconv.ParseFloat(record[3], 64)
		finishingDelta, _ := strconv.ParseFloat(record[6], 64)
		concededDelta, _ := strconv.ParseFloat(record[7], 64)

		team := p.findTeamByName(teamName)
		if team == nil {
			continue
		}

		if isHome {
			team.XGForHome = xgFor
			team.XGAgainstHome = xgAgainst
			team.FinishingDeltaHome = finishingDelta
			team.ConcededDeltaHome = concededDelta
		} else {
			team.XGForAway = xgFor
			team.XGAgainstAway = xgAgainst
			team.FinishingDeltaAway = finishingDelta
			team.ConcededDeltaAway = concededDelta
		}
	}
	return nil
}
func (p *Predictor) getXGForMatch(team *Team, isHome bool) (xgFor, xgAgainst, finishingDelta, concededDelta float64) {
	if isHome {
		// Si no hay datos de local, usar 0 (se manejará después)
		return team.XGForHome, team.XGAgainstHome, team.FinishingDeltaHome, team.ConcededDeltaHome
	}
	return team.XGForAway, team.XGAgainstAway, team.FinishingDeltaAway, team.ConcededDeltaAway
}


func (p *Predictor) LoadWDLData(wdlFile string, location string) error {
	file, err := os.Open(wdlFile)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	for i, record := range records {
		if i == 0 {
			continue // Saltar cabecera
		}
		if len(record) < 5 {
			continue
		}

		teamName := record[0]
		winPct, _ := strconv.ParseFloat(strings.TrimSuffix(record[2], "%"), 64)
		drawPct, _ := strconv.ParseFloat(strings.TrimSuffix(record[3], "%"), 64)
		lossPct, _ := strconv.ParseFloat(strings.TrimSuffix(record[4], "%"), 64)

		// Buscar el equipo
		team := p.findTeamByName(teamName)
		if team == nil {
			continue
		}

		// Asignar segun localia
		switch location {
		case "home":
			team.WinHome = winPct / 100
			team.DrawHome = drawPct / 100
			team.LossHome = lossPct / 100
		case "away":
			team.WinAway = winPct / 100
			team.DrawAway = drawPct / 100
			team.LossAway = lossPct / 100
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
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	for i, record := range records {
		if i == 0 {
			continue // Saltar cabecera
		}
		if len(record) < 11 {
			continue
		}

		teamName := record[0]
		matches, _ := strconv.Atoi(record[1])
		avgTaken, _ := strconv.ParseFloat(record[2], 64)
		avgConceded, _ := strconv.ParseFloat(record[3], 64)

		// Crear estructura con los datos
		stats := &CornerStats{
			MatchesPlayed:      matches,
			AvgCornersTaken:    avgTaken,
			AvgCornersConceded: avgConceded,
			OverPercent:        make(map[float64]float64),
			UnderPercent:       make(map[float64]float64),
		}

		// Cargar porcentajes (columnas 4-11)
		thresholds := []float64{7.5, 8.5, 9.5, 10.5}
		for j, th := range thresholds {
			overIdx := 4 + j*2
			underIdx := 5 + j*2
			if overIdx < len(record) && underIdx < len(record) {
				over, _ := strconv.ParseFloat(strings.TrimSuffix(record[overIdx], "%"), 64)
				under, _ := strconv.ParseFloat(strings.TrimSuffix(record[underIdx], "%"), 64)
				stats.OverPercent[th] = over / 100
				stats.UnderPercent[th] = under / 100
			}
		}

		// Buscar equipo y asignar
		team := p.findTeamByName(teamName)
		if team == nil {
			continue
		}

		if location == "home" {
			team.HomeCorners = stats
		} else {
			team.AwayCorners = stats
		}
	}
	return nil
}

func (p *Predictor) findTeamByName(name string) *Team {
	// Limpiar nombre (quitar espacios extras, normalizar)
	cleanName := strings.TrimSpace(name)

	// Busqueda exacta primero
	for _, team := range p.Teams {
		if strings.EqualFold(team.Name, cleanName) {
			return team
		}
	}

	// Busqueda parcial si no encuentra
	for _, team := range p.Teams {
		if strings.Contains(strings.ToLower(team.Name), strings.ToLower(cleanName)) ||
			strings.Contains(strings.ToLower(cleanName), strings.ToLower(team.Name)) {
			return team
		}
	}

	return nil
}
