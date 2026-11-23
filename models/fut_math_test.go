package models

import (
	"math"
	"testing"
)

func TestMathInvariants(t *testing.T) {
	p := NewPredictor()

	home := &Team{
		ID: "H", Name: "Home FC", League: "LaLiga", PowerRating: 75, Style: "BANDA",
		GoalsForAvg: 1.8, GoalsAgainstAvg: 0.9, AvgCornersFor: 5.5, AvgCornersAgainst: 4.0,
		XGForHome: 1.9, XGAgainstHome: 0.85,
		WinHome: 0.65, DrawHome: 0.25, LossHome: 0.10,
		BTTS1H: 0.25, BTTS2H: 0.35,
	}
	away := &Team{
		ID: "A", Name: "Away FC", League: "LaLiga", PowerRating: 68, Style: "MIXTO",
		GoalsForAvg: 1.3, GoalsAgainstAvg: 1.2, AvgCornersFor: 4.5, AvgCornersAgainst: 5.0,
		XGForAway: 1.2, XGAgainstAway: 1.35,
		WinAway: 0.35, DrawAway: 0.30, LossAway: 0.35,
		BTTS1H: 0.20, BTTS2H: 0.30,
	}

	// 1. 1X2 debe sumar exactamente 1.0
	out := p.PredictOutcome(home, away)
	sum1X2 := out.HomeWinProb + out.DrawProb + out.AwayWinProb
	if math.Abs(sum1X2-1.0) > 1e-9 {
		t.Fatalf("suma 1X2 != 1.0: %.12f", sum1X2)
	}

	// 2. Hándicap: HomeProb + AwayProb == 1.0 y -1.5 más difícil que +1.5
	hNeg := p.CalculateHandicap(home, away, -1.5)
	hPos := p.CalculateHandicap(home, away, +1.5)
	if math.Abs((hNeg.HomeProb+hNeg.AwayProb)-1.0) > 1e-9 {
		t.Fatalf("suma hándicap != 1.0: %.12f", hNeg.HomeProb+hNeg.AwayProb)
	}
	if hNeg.HomeProb >= hPos.HomeProb {
		t.Fatalf("hándicap -1.5 (%.3f) no puede tener mayor prob que +1.5 (%.3f)", hNeg.HomeProb, hPos.HomeProb)
	}

	// 3. BTTS en rango válido y positivo (usando 1H/2H cuando BTTSOverall == 0)
	btts := p.CalculateBTTSProbability(home, away)
	if btts.Probability <= 0 || btts.Probability >= 1 {
		t.Fatalf("BTTS fuera de rango: %.4f", btts.Probability)
	}

	// 4. Córners: monotonicidad Over75 >= Over85 >= Over95 >= Over105
	corners := p.CalculateCornerExpectation(home, away)
	if !(corners.Over75 >= corners.Over85 && corners.Over85 >= corners.Over95 && corners.Over95 >= corners.Over105) {
		t.Fatalf("monotonicidad Over rota: %.4f %.4f %.4f %.4f",
			corners.Over75, corners.Over85, corners.Over95, corners.Over105)
	}

	// 5. Edge y Kelly deben tener el mismo signo en apuestas +EV y -EV
	if edge := p.Edge(0.60, 2.00); math.Abs(edge-20.0) > 1e-9 {
		t.Fatalf("Edge(0.60, 2.00) esperado +20.0, obtenido %.4f", edge)
	}
	if edge := p.Edge(0.60, 1.20); edge >= 0 {
		t.Fatalf("Edge(0.60, 1.20) debía ser negativo, obtenido %.4f", edge)
	}
	ok, kelly, edge := p.RecommendBet(0.60, 2.00, 0.05)
	if !ok || kelly <= 0 || edge <= 0 {
		t.Fatalf("RecommendBet inconsistente en +EV: ok=%v kelly=%.4f edge=%.4f", ok, kelly, edge)
	}
}
