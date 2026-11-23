package models

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestAprilCSVsFromDataDir(t *testing.T) {
	p := NewPredictor()
	dataDir := filepath.Join("..", "data")

	equiposPath := filepath.Join(dataDir, "equipos.csv")
	if _, err := os.Stat(equiposPath); err == nil {
		if err := p.LoadTeamsFromCSV(equiposPath); err != nil {
			t.Fatalf("error cargando equipos.csv: %v", err)
		}
	}

	btts1HPath := filepath.Join(dataDir, "team_BTTS_1H_Overall_England Premier League.csv")
	if err := p.LoadBTTSData(btts1HPath, "1H"); err != nil {
		t.Fatalf("error en LoadBTTSData: %v", err)
	}
	for _, name := range []string{"Burnley", "Bournemouth", "Newcastle", "Manchester Utd", "Ipswich"} {
		if team := p.findTeamByName(name); team != nil {
			fmt.Printf("[BTTS 1H] %s -> BTTS1H=%.2f\n", team.Name, team.BTTS1H)
		}
	}

	xgHomePath := filepath.Join(dataDir, "team_xG_FT_Home_Europe Champions League.csv")
	if err := p.loadXGFile(xgHomePath, true); err != nil {
		t.Fatalf("error en loadXGFile: %v", err)
	}
	shown := 0
	for _, team := range p.Teams {
		if team.XGForHome != 0 || team.XGAgainstHome != 0 || team.Competition == "Champions" {
			fmt.Printf("[xG Home] %s | Comp=%q | For=%.2f | Ag=%.2f | FinDelta=%.2f | ConcDelta=%.2f\n",
				team.Name, team.Competition, team.XGForHome, team.XGAgainstHome, team.FinishingDeltaHome, team.ConcededDeltaHome)
			shown++
			if shown >= 5 {
				break
			}
		}
	}

	wdlHomePath := filepath.Join(dataDir, "team_WDL_FT_Home_Europe Champions League.csv")
	if err := p.LoadWDLData(wdlHomePath, "home"); err != nil {
		t.Fatalf("error en LoadWDLData: %v", err)
	}
	shown = 0
	for _, team := range p.Teams {
		if team.WinHome != 0 || team.DrawHome != 0 || team.LossHome != 0 {
			fmt.Printf("[WDL Home] %s | W=%.3f | D=%.3f | L=%.3f (suma=%.3f)\n",
				team.Name, team.WinHome, team.DrawHome, team.LossHome, team.WinHome+team.DrawHome+team.LossHome)
			shown++
			if shown >= 5 {
				break
			}
		}
	}
}

func TestCornersFromDataDir(t *testing.T) {
	p := NewPredictor()
	dataDir := filepath.Join("..", "data")

	equiposPath := filepath.Join(dataDir, "equipos.csv")
	if _, err := os.Stat(equiposPath); err == nil {
		if err := p.LoadTeamsFromCSV(equiposPath); err != nil {
			t.Fatalf("error cargando equipos.csv: %v", err)
		}
	}

	homeCornersPath := filepath.Join(dataDir, "team_Corners_FT_Home_Europe Champions League.csv")
	awayCornersPath := filepath.Join(dataDir, "team_Corners_FT_Away_Europe Champions League.csv")

	if _, err := os.Stat(homeCornersPath); err == nil {
		if err := p.LoadCornersData(homeCornersPath, "home"); err != nil {
			t.Fatalf("error en LoadCornersData (home): %v", err)
		}
	} else {
		fmt.Printf("[Corners Home] No encontrado: %s\n", homeCornersPath)
	}

	if _, err := os.Stat(awayCornersPath); err == nil {
		if err := p.LoadCornersData(awayCornersPath, "away"); err != nil {
			t.Fatalf("error en LoadCornersData (away): %v", err)
		}
	} else {
		fmt.Printf("[Corners Away] No encontrado: %s\n", awayCornersPath)
	}

	shown := 0
	for _, team := range p.Teams {
		if team.HomeCorners != nil || team.AwayCorners != nil {
			var hTaken, aTaken float64
			if team.HomeCorners != nil {
				hTaken = team.HomeCorners.AvgCornersTaken
			}
			if team.AwayCorners != nil {
				aTaken = team.AwayCorners.AvgCornersTaken
			}
			fmt.Printf("[Corners] %s | HomeTaken=%.2f | AwayTaken=%.2f\n", team.Name, hTaken, aTaken)
			shown++
			if shown >= 5 {
				break
			}
		}
	}

	home := p.findTeamByName("Manchester City")
	away := p.findTeamByName("Liverpool")
	if home != nil && away != nil {
		pred := p.CalculateCornerExpectation(home, away)
		fmt.Printf("[Pred Corners] %s (local) vs %s (visita) -> Esp=%.2f (H=%.2f, A=%.2f) | Over7.5=%.1f%% | Over8.5=%.1f%% | Over9.5=%.1f%% | Over10.5=%.1f%%\n",
			home.Name, away.Name, pred.TotalExpected, pred.HomeExpected, pred.AwayExpected,
			pred.Over75*100, pred.Over85*100, pred.Over95*100, pred.Over105*100)
	}
}
