package models

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type leagueSyncSpec struct {
	FileSuffix string
	CsvLeague  string
	IsChamp    bool
}

var defaultSyncLeagues = []leagueSyncSpec{
	{FileSuffix: "Spain LaLiga", CsvLeague: "LaLiga"},
	{FileSuffix: "England Premier League", CsvLeague: "Premier"},
	{FileSuffix: "Germany Bundesliga", CsvLeague: "Bundesliga"},
	{FileSuffix: "Italy Serie A", CsvLeague: "SerieA"},
	{FileSuffix: "France Ligue 1", CsvLeague: "Ligue1"},
	{FileSuffix: "Brazil Brazilian League", CsvLeague: "Brasileirao"},
	{FileSuffix: "Mexico Liga MX", CsvLeague: "Liga MX"},
	{FileSuffix: "Europe Champions League", CsvLeague: "Champions", IsChamp: true},
}

type syncedTeamRow struct {
	ID                string
	Name              string
	League            string
	PowerRating       float64
	GAx               float64
	GSAx              float64
	Style             string
	AvgCornersFor     float64
	AvgCornersAgainst float64
	BTTSRate          float64
	GoalsForAvg       float64
	GoalsAgainstAvg   float64
	Competition       string
	Existed           bool
}

type teamMetricAccum struct {
	DisplayName string
	DomLeague   string
	InChampions bool

	CornersForHomeSum float64
	CornersForHomeN   int
	CornersAgHomeSum  float64
	CornersAgHomeN    int
	CornersForAwaySum float64
	CornersForAwayN   int
	CornersAgAwaySum  float64
	CornersAgAwayN    int

	BTTS1H    float64
	HasBTTS1H bool
	BTTS2H    float64
	HasBTTS2H bool

	WinHome, DrawHome, LossHome float64
	HasWDLHome                  bool
	WinAway, DrawAway, LossAway float64
	HasWDLAway                  bool

	XGForHome, XGAgHome, GFHome, GAHome float64
	HasXGHome                           bool
	XGForAway, XGAgAway, GFAway, GAAway float64
	HasXGAway                           bool
}

// SyncEquiposCSV genera o actualiza dataDir/equipos.csv combinando los CSVs de Footiqo ya descargados.
func SyncEquiposCSV(dataDir string) error {
	if strings.TrimSpace(dataDir) == "" {
		return fmt.Errorf("dataDir vacío")
	}

	equiposPath := filepath.Join(dataDir, "equipos.csv")
	rows, byNorm := loadExistingEquiposRows(equiposPath)

	accums := make(map[string]*teamMetricAccum)
	getAcc := func(rawName string) *teamMetricAccum {
		norm := normalizeTeamName(rawName)
		if norm == "" {
			return nil
		}
		a, ok := accums[norm]
		if !ok {
			a = &teamMetricAccum{DisplayName: strings.TrimSpace(rawName)}
			accums[norm] = a
		}
		return a
	}

	csvFilesFound := 0
	for _, lg := range defaultSyncLeagues {
		// 1. Corners Home / Away
		for _, isHome := range []bool{true, false} {
			loc := "Away"
			if isHome {
				loc = "Home"
			}
			path := filepath.Join(dataDir, fmt.Sprintf("team_Corners_FT_%s_%s.csv", loc, lg.FileSuffix))
			if parsed := syncReadCornersFile(path); len(parsed) > 0 {
				csvFilesFound++
				for _, item := range parsed {
					a := getAcc(item.team)
					if a == nil {
						continue
					}
					if lg.IsChamp {
						a.InChampions = true
					} else if a.DomLeague == "" {
						a.DomLeague = lg.CsvLeague
					}
					if isHome {
						if item.hasTaken {
							a.CornersForHomeSum += item.taken
							a.CornersForHomeN++
						}
						if item.hasConceded {
							a.CornersAgHomeSum += item.conceded
							a.CornersAgHomeN++
						}
					} else {
						if item.hasTaken {
							a.CornersForAwaySum += item.taken
							a.CornersForAwayN++
						}
						if item.hasConceded {
							a.CornersAgAwaySum += item.conceded
							a.CornersAgAwayN++
						}
					}
				}
			}
		}

		// 2. BTTS 1H / 2H
		for _, period := range []string{"1H", "2H"} {
			path := filepath.Join(dataDir, fmt.Sprintf("team_BTTS_%s_Overall_%s.csv", period, lg.FileSuffix))
			if parsed := syncReadBTTSFile(path); len(parsed) > 0 {
				csvFilesFound++
				for _, item := range parsed {
					a := getAcc(item.team)
					if a == nil {
						continue
					}
					if lg.IsChamp {
						a.InChampions = true
					} else if a.DomLeague == "" {
						a.DomLeague = lg.CsvLeague
					}
					if period == "1H" {
						a.BTTS1H = item.yesProp
						a.HasBTTS1H = true
					} else {
						a.BTTS2H = item.yesProp
						a.HasBTTS2H = true
					}
				}
			}
		}

		// 3. WDL Home / Away
		for _, isHome := range []bool{true, false} {
			loc := "Away"
			if isHome {
				loc = "Home"
			}
			path := filepath.Join(dataDir, fmt.Sprintf("team_WDL_FT_%s_%s.csv", loc, lg.FileSuffix))
			if parsed := syncReadWDLFile(path); len(parsed) > 0 {
				csvFilesFound++
				for _, item := range parsed {
					a := getAcc(item.team)
					if a == nil {
						continue
					}
					if lg.IsChamp {
						a.InChampions = true
					} else if a.DomLeague == "" {
						a.DomLeague = lg.CsvLeague
					}
					if isHome {
						a.WinHome, a.DrawHome, a.LossHome = item.w, item.d, item.l
						a.HasWDLHome = true
					} else {
						a.WinAway, a.DrawAway, a.LossAway = item.w, item.d, item.l
						a.HasWDLAway = true
					}
				}
			}
		}

		// 4. xG Home / Away
		for _, isHome := range []bool{true, false} {
			loc := "Away"
			if isHome {
				loc = "Home"
			}
			path := filepath.Join(dataDir, fmt.Sprintf("team_xG_FT_%s_%s.csv", loc, lg.FileSuffix))
			if parsed := syncReadXGFile(path); len(parsed) > 0 {
				csvFilesFound++
				for _, item := range parsed {
					a := getAcc(item.team)
					if a == nil {
						continue
					}
					if lg.IsChamp {
						a.InChampions = true
					} else if a.DomLeague == "" {
						a.DomLeague = lg.CsvLeague
					}
					if isHome {
						a.XGForHome, a.XGAgHome = item.xgFor, item.xgAg
						a.GFHome, a.GAHome = item.gf, item.ga
						a.HasXGHome = true
					} else {
						a.XGForAway, a.XGAgAway = item.xgFor, item.xgAg
						a.GFAway, a.GAAway = item.gf, item.ga
						a.HasXGAway = true
					}
				}
			}
		}
	}

	// Si no hay CSVs descargados todavía, no sobrescribir nada
	if csvFilesFound == 0 || len(accums) == 0 {
		return nil
	}

	usedIDs := make(map[string]bool, len(rows)+len(accums))
	for _, r := range rows {
		if r.ID == "" || usedIDs[r.ID] {
			r.ID = generateUniqueTeamID(r.Name, usedIDs)
		}
		usedIDs[r.ID] = true
	}

	// Mezclar métricas nuevas en equipos existentes y crear los faltantes
	norms := make([]string, 0, len(accums))
	for k := range accums {
		norms = append(norms, k)
	}
	sort.Strings(norms)

	for _, norm := range norms {
		a := accums[norm]
		row := findSyncedRow(norm, byNorm)
		if row == nil {
			league := a.DomLeague
			if league == "" {
				league = "Champions"
			}
			row = &syncedTeamRow{
				ID:     generateUniqueTeamID(a.DisplayName, usedIDs),
				Name:   a.DisplayName,
				League: league,
				Style:  "MIXTO",
			}
			usedIDs[row.ID] = true
			rows = append(rows, row)
			byNorm[norm] = row
		} else if row.League == "" && a.DomLeague != "" {
			row.League = a.DomLeague
		}

		if a.InChampions && !strings.Contains(strings.ToLower(row.Competition), "champions") {
			row.Competition = "Champions"
		}

		applyAccumulatedMetrics(row, a)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].League != rows[j].League {
			return rows[i].League < rows[j].League
		}
		return rows[i].Name < rows[j].Name
	})

	tmpPath := equiposPath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("error creando %s: %w", tmpPath, err)
	}

	writer := csv.NewWriter(file)
	header := []string{
		"ID", "Nombre", "Liga", "PowerRating", "GAx", "GSAx", "Estilo",
		"AvgCornersFor", "AvgCornersAgainst", "BTTSRate", "GoalsForAvg", "GoalsAgainstAvg", "Competition",
	}
	if err := writer.Write(header); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return err
	}

	for _, r := range rows {
		rec := []string{
			r.ID,
			r.Name,
			r.League,
			formatSyncFloat(r.PowerRating),
			formatSyncFloat(r.GAx),
			formatSyncFloat(r.GSAx),
			r.Style,
			formatSyncFloat(r.AvgCornersFor),
			formatSyncFloat(r.AvgCornersAgainst),
			formatSyncFloat(r.BTTSRate),
			formatSyncFloat(r.GoalsForAvg),
			formatSyncFloat(r.GoalsAgainstAvg),
			r.Competition,
		}
		if err := writer.Write(rec); err != nil {
			file.Close()
			os.Remove(tmpPath)
			return err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, equiposPath)
}

// ReloadFromDataDir recarga equipos.csv y todas las estadísticas de las 8 ligas en memoria.
func (p *Predictor) ReloadFromDataDir(dataDir string) error {
	fresh := NewPredictor()
	if err := fresh.LoadTeamsFromCSV(filepath.Join(dataDir, "equipos.csv")); err != nil {
		return err
	}

	for _, lg := range defaultSyncLeagues {
		liga := lg.FileSuffix
		_ = fresh.LoadBTTSData(filepath.Join(dataDir, fmt.Sprintf("team_BTTS_1H_Overall_%s.csv", liga)), "1H")
		_ = fresh.LoadBTTSData(filepath.Join(dataDir, fmt.Sprintf("team_BTTS_2H_Overall_%s.csv", liga)), "2H")

		_ = fresh.LoadXGSeparatedData(
			filepath.Join(dataDir, fmt.Sprintf("team_xG_FT_Home_%s.csv", liga)),
			filepath.Join(dataDir, fmt.Sprintf("team_xG_FT_Away_%s.csv", liga)),
		)

		_ = fresh.LoadWDLData(filepath.Join(dataDir, fmt.Sprintf("team_WDL_FT_Home_%s.csv", liga)), "home")
		_ = fresh.LoadWDLData(filepath.Join(dataDir, fmt.Sprintf("team_WDL_FT_Away_%s.csv", liga)), "away")

		_ = fresh.LoadCornersData(filepath.Join(dataDir, fmt.Sprintf("team_Corners_FT_Home_%s.csv", liga)), "home")
		_ = fresh.LoadCornersData(filepath.Join(dataDir, fmt.Sprintf("team_Corners_FT_Away_%s.csv", liga)), "away")
	}

	p.Teams = fresh.Teams
	return nil
}

func formatSyncFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func findSyncedRow(norm string, byNorm map[string]*syncedTeamRow) *syncedTeamRow {
	if r, ok := byNorm[norm]; ok {
		return r
	}
	if len(norm) >= 5 {
		var match *syncedTeamRow
		for k, r := range byNorm {
			if len(k) < 5 {
				continue
			}
			if strings.Contains(k, norm) || strings.Contains(norm, k) {
				if match != nil {
					return nil
				}
				match = r
			}
		}
		return match
	}
	return nil
}

func applyAccumulatedMetrics(row *syncedTeamRow, a *teamMetricAccum) {
	// Corners promedio
	if avgFor, ok := avgPairs(a.CornersForHomeSum, a.CornersForHomeN, a.CornersForAwaySum, a.CornersForAwayN); ok {
		row.AvgCornersFor = avgFor
	} else if !row.Existed && row.AvgCornersFor <= 0 {
		row.AvgCornersFor = 5.20
	}

	if avgAg, ok := avgPairs(a.CornersAgHomeSum, a.CornersAgHomeN, a.CornersAgAwaySum, a.CornersAgAwayN); ok {
		row.AvgCornersAgainst = avgAg
	} else if !row.Existed && row.AvgCornersAgainst <= 0 {
		row.AvgCornersAgainst = 4.50
	}

	// BTTSRate por unión 1H/2H si ambos existen
	if a.HasBTTS1H && a.HasBTTS2H {
		b1 := clampFloat(a.BTTS1H, 0, 1)
		b2 := clampFloat(a.BTTS2H, 0, 1)
		row.BTTSRate = clampFloat(1.0-(1.0-b1)*(1.0-b2), 0.10, 0.95)
	} else if !row.Existed && row.BTTSRate <= 0 {
		switch {
		case a.HasBTTS1H:
			row.BTTSRate = clampFloat(a.BTTS1H*1.5, 0.15, 0.90)
		case a.HasBTTS2H:
			row.BTTSRate = clampFloat(a.BTTS2H*1.4, 0.15, 0.90)
		default:
			row.BTTSRate = 0.50
		}
	}

	// Goles y xG si hay CSV de xG
	if a.HasXGHome || a.HasXGAway {
		if gf, ok := avgOpt(a.GFHome, a.HasXGHome && a.GFHome > 0, a.GFAway, a.HasXGAway && a.GFAway > 0); ok {
			row.GoalsForAvg = gf
		}
		if ga, ok := avgOpt(a.GAHome, a.HasXGHome && a.GAHome > 0, a.GAAway, a.HasXGAway && a.GAAway > 0); ok {
			row.GoalsAgainstAvg = ga
		}
		if !row.Existed {
			if xgf, ok := avgOpt(a.XGForHome, a.HasXGHome && a.XGForHome > 0, a.XGForAway, a.HasXGAway && a.XGForAway > 0); ok {
				row.GAx = xgf
			}
			if xga, ok := avgOpt(a.XGAgHome, a.HasXGHome && a.XGAgHome > 0, a.XGAgAway, a.HasXGAway && a.XGAgAway > 0); ok {
				row.GSAx = xga
			}
		}
	}

	// Estimaciones iniciales solo para equipos NUEVOS sin xG o PowerRating previo
	if !row.Existed {
		winAvg, hasW := avgOpt(a.WinHome, a.HasWDLHome, a.WinAway, a.HasWDLAway)
		drawAvg, hasD := avgOpt(a.DrawHome, a.HasWDLHome, a.DrawAway, a.HasWDLAway)
		lossAvg, hasL := avgOpt(a.LossHome, a.HasWDLHome, a.LossAway, a.HasWDLAway)

		if hasW && hasD {
			ppg := 3.0*winAvg + drawAvg
			row.PowerRating = clampFloat(70.0+(ppg/3.0)*26.0, 70.0, 96.0)
			if row.GoalsForAvg <= 0 {
				row.GoalsForAvg = clampFloat(0.80+1.40*winAvg+0.40*drawAvg, 0.70, 2.80)
			}
			if row.GoalsAgainstAvg <= 0 && hasL {
				row.GoalsAgainstAvg = clampFloat(0.80+1.40*lossAvg+0.40*drawAvg, 0.60, 2.50)
			}
		}
		if row.PowerRating <= 0 {
			row.PowerRating = 80.0
		}
		if row.GoalsForAvg <= 0 {
			row.GoalsForAvg = 1.35
		}
		if row.GoalsAgainstAvg <= 0 {
			row.GoalsAgainstAvg = 1.30
		}
		if row.GAx <= 0 {
			row.GAx = row.GoalsForAvg
		}
		if row.GSAx <= 0 {
			row.GSAx = row.GoalsAgainstAvg
		}
		switch {
		case row.AvgCornersFor >= 6.0:
			row.Style = "BANDA"
		case row.AvgCornersFor > 0 && row.AvgCornersFor <= 4.8:
			row.Style = "CENTRO"
		default:
			row.Style = "MIXTO"
		}
	}
}

func avgPairs(s1 float64, n1 int, s2 float64, n2 int) (float64, bool) {
	var parts []float64
	if n1 > 0 {
		parts = append(parts, s1/float64(n1))
	}
	if n2 > 0 {
		parts = append(parts, s2/float64(n2))
	}
	if len(parts) == 0 {
		return 0, false
	}
	sum := 0.0
	for _, v := range parts {
		sum += v
	}
	return sum / float64(len(parts)), true
}

func avgOpt(v1 float64, ok1 bool, v2 float64, ok2 bool) (float64, bool) {
	switch {
	case ok1 && ok2:
		return (v1 + v2) / 2.0, true
	case ok1:
		return v1, true
	case ok2:
		return v2, true
	default:
		return 0, false
	}
}

func generateUniqueTeamID(name string, used map[string]bool) string {
	var alnum []rune
	for _, r := range strings.ToUpper(normalizeTeamName(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			alnum = append(alnum, r)
		}
	}
	base := string(alnum)
	for len(base) < 3 {
		base += "X"
	}
	cand := base[:3]
	if !used[cand] {
		return cand
	}
	if len(base) >= 4 {
		cand4 := base[:2] + string(base[len(base)-1])
		if !used[cand4] {
			return cand4
		}
	}
	for n := 2; n < 100; n++ {
		c := base[:2] + strconv.Itoa(n)
		if !used[c] {
			return c
		}
	}
	return base[:3] + "Z"
}

func loadExistingEquiposRows(path string) ([]*syncedTeamRow, map[string]*syncedTeamRow) {
	byNorm := make(map[string]*syncedTeamRow)
	file, err := os.Open(path)
	if err != nil {
		return nil, byNorm
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil || len(records) <= 1 {
		return nil, byNorm
	}

	headers := mapHeaders(records[0])
	headerCount := len(records[0])
	var rows []*syncedTeamRow

	for i, record := range records {
		if i == 0 || isEmptyRecord(record) || len(record) < 12 {
			continue
		}
		aligned := len(record) == headerCount
		idxOr := func(def int, keys ...string) int {
			if aligned {
				if idx, ok := lookupHeader(headers, keys...); ok {
					return idx
				}
			}
			return def
		}

		name := strings.TrimSpace(record[idxOr(1, "nombre", "name", "team", "equipo")])
		if name == "" {
			continue
		}
		norm := normalizeTeamName(name)
		if _, dup := byNorm[norm]; dup {
			continue
		}

		pr, err1 := parseCleanFloat(record[idxOr(3, "powerrating", "power rating")])
		gax, err2 := parseCleanFloat(record[idxOr(4, "gax")])
		gsax, err3 := parseCleanFloat(record[idxOr(5, "gsax")])
		cf, err4 := parseCleanFloat(record[idxOr(7, "avgcornersfor", "avg corners for")])
		ca, err5 := parseCleanFloat(record[idxOr(8, "avgcornersagainst", "avg corners against")])
		btts, err6 := parseFlexibleProportion(record[idxOr(9, "bttsrate", "btts rate")])
		gf, err7 := parseCleanFloat(record[idxOr(10, "goalsforavg", "goals for avg")])
		ga, err8 := parseCleanFloat(record[idxOr(11, "goalsagainstavg", "goals against avg")])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil || err7 != nil || err8 != nil {
			continue
		}

		comp := ""
		if idx, ok := lookupHeader(headers, "competition", "competicion"); aligned && ok && idx < len(record) {
			comp = strings.TrimSpace(record[idx])
		} else if len(record) == 13 {
			if _, err := parseCleanFloat(record[12]); err != nil {
				comp = strings.TrimSpace(record[12])
			}
		} else if len(record) >= 14 {
			comp = strings.TrimSpace(record[13])
		}

		row := &syncedTeamRow{
			ID:                strings.TrimSpace(record[idxOr(0, "id")]),
			Name:              name,
			League:            strings.TrimSpace(record[idxOr(2, "liga", "league")]),
			PowerRating:       pr,
			GAx:               gax,
			GSAx:              gsax,
			Style:             strings.ToUpper(strings.TrimSpace(record[idxOr(6, "estilo", "style")])),
			AvgCornersFor:     cf,
			AvgCornersAgainst: ca,
			BTTSRate:          btts,
			GoalsForAvg:       gf,
			GoalsAgainstAvg:   ga,
			Competition:       comp,
			Existed:           true,
		}
		rows = append(rows, row)
		byNorm[norm] = row
	}
	return rows, byNorm
}

type syncCornerItem struct {
	team        string
	taken       float64
	hasTaken    bool
	conceded    float64
	hasConceded bool
}

func syncReadCornersFile(path string) []syncCornerItem {
	records, headers, ok := syncReadCSV(path)
	if !ok {
		return nil
	}
	headerCount := len(records[0])
	var out []syncCornerItem
	for i, rec := range records {
		if i == 0 || isEmptyRecord(rec) || len(rec) < 4 {
			continue
		}
		offset := 0
		if len(rec) >= 15 {
			offset = 3
		}
		teamIdx, takenIdx, concIdx := offset, 2+offset, 3+offset
		if len(rec) == headerCount {
			if idx, found := lookupHeader(headers, "team", "equipo"); found {
				teamIdx = idx
			}
			if idx, found := lookupHeader(headers, "avg corners taken", "corners taken"); found {
				takenIdx = idx
			}
			if idx, found := lookupHeader(headers, "avg corners conceded", "corners conceded"); found {
				concIdx = idx
			}
		}
		if len(rec) <= teamIdx || len(rec) <= takenIdx || len(rec) <= concIdx {
			continue
		}
		name := strings.TrimSpace(rec[teamIdx])
		if name == "" {
			continue
		}
		t, errT := parseCleanFloat(rec[takenIdx])
		c, errC := parseCleanFloat(rec[concIdx])
		if errT != nil && errC != nil {
			continue
		}
		out = append(out, syncCornerItem{
			team:        name,
			taken:       t,
			hasTaken:    errT == nil && t >= 0,
			conceded:    c,
			hasConceded: errC == nil && c >= 0,
		})
	}
	return out
}

type syncBTTSItem struct {
	team    string
	yesProp float64
}

func syncReadBTTSFile(path string) []syncBTTSItem {
	records, headers, ok := syncReadCSV(path)
	if !ok {
		return nil
	}
	headerCount := len(records[0])
	var out []syncBTTSItem
	for i, rec := range records {
		if i == 0 || isEmptyRecord(rec) {
			continue
		}
		teamIdx, bttsIdx := 0, 2
		if len(rec) >= 7 {
			teamIdx, bttsIdx = 3, 5
		}
		if len(rec) == headerCount {
			if idx, found := lookupHeader(headers, "team", "equipo"); found {
				teamIdx = idx
			}
			if idx, found := lookupHeader(headers, "btts yes %", "btts yes", "btts yes (%)"); found {
				bttsIdx = idx
			}
		}
		if len(rec) <= teamIdx || len(rec) <= bttsIdx {
			continue
		}
		name := strings.TrimSpace(rec[teamIdx])
		prop, err := parsePercentProportion(rec[bttsIdx])
		if name == "" || err != nil {
			continue
		}
		out = append(out, syncBTTSItem{team: name, yesProp: prop})
	}
	return out
}

type syncWDLItem struct {
	team    string
	w, d, l float64
}

func syncReadWDLFile(path string) []syncWDLItem {
	records, headers, ok := syncReadCSV(path)
	if !ok {
		return nil
	}
	headerCount := len(records[0])
	var out []syncWDLItem
	for i, rec := range records {
		if i == 0 || isEmptyRecord(rec) || len(rec) < 5 {
			continue
		}
		teamIdx, wIdx, dIdx, lIdx := 0, 2, 3, 4
		if len(rec) >= 8 {
			teamIdx, wIdx, dIdx, lIdx = 3, 5, 6, 7
		}
		if len(rec) == headerCount {
			if idx, found := lookupHeader(headers, "team", "equipo"); found {
				teamIdx = idx
			}
			if idx, found := lookupHeader(headers, "win %", "win", "win (%)"); found {
				wIdx = idx
			}
			if idx, found := lookupHeader(headers, "draw %", "draw", "draw (%)"); found {
				dIdx = idx
			}
			if idx, found := lookupHeader(headers, "loss %", "loss", "loss (%)"); found {
				lIdx = idx
			}
		}
		if len(rec) <= teamIdx || len(rec) <= wIdx || len(rec) <= dIdx || len(rec) <= lIdx {
			continue
		}
		name := strings.TrimSpace(rec[teamIdx])
		w, errW := parsePercentProportion(rec[wIdx])
		d, errD := parsePercentProportion(rec[dIdx])
		l, errL := parsePercentProportion(rec[lIdx])
		if name == "" || errW != nil || errD != nil || errL != nil {
			continue
		}
		out = append(out, syncWDLItem{team: name, w: w, d: d, l: l})
	}
	return out
}

type syncXGItem struct {
	team        string
	xgFor, xgAg float64
	gf, ga      float64
}

func syncReadXGFile(path string) []syncXGItem {
	records, headers, ok := syncReadCSV(path)
	if !ok {
		return nil
	}
	headerCount := len(records[0])
	var out []syncXGItem
	for i, rec := range records {
		if i == 0 || isEmptyRecord(rec) || len(rec) < 11 {
			continue
		}
		periodIdx := 0
		teamIdx, xgfIdx, xgaIdx, gfIdx, gaIdx := 3, 5, 6, 7, 8
		if len(rec) >= 12 {
			gfIdx, gaIdx = 8, 9
		}
		if len(rec) == headerCount {
			if idx, found := lookupHeader(headers, "period", "timeframe"); found {
				periodIdx = idx
			}
			if idx, found := lookupHeader(headers, "team", "equipo"); found {
				teamIdx = idx
			}
			if idx, found := lookupHeader(headers, "avg xg for", "xg for"); found {
				xgfIdx = idx
			}
			if idx, found := lookupHeader(headers, "avg xg against", "xg against"); found {
				xgaIdx = idx
			}
			if idx, found := lookupHeader(headers, "avg goals for", "goals for"); found {
				gfIdx = idx
			}
			if idx, found := lookupHeader(headers, "avg goals against", "goals against"); found {
				gaIdx = idx
			}
		}
		if periodIdx < len(rec) {
			pVal := strings.TrimSpace(rec[periodIdx])
			if pVal != "" && !strings.EqualFold(pVal, "Current season") {
				continue
			}
		}
		if len(rec) <= teamIdx || len(rec) <= xgfIdx || len(rec) <= xgaIdx || len(rec) <= gfIdx || len(rec) <= gaIdx {
			continue
		}
		name := strings.TrimSpace(rec[teamIdx])
		xgf, err1 := parseCleanFloat(rec[xgfIdx])
		xga, err2 := parseCleanFloat(rec[xgaIdx])
		gf, err3 := parseCleanFloat(rec[gfIdx])
		ga, err4 := parseCleanFloat(rec[gaIdx])
		if name == "" || err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			continue
		}
		out = append(out, syncXGItem{team: name, xgFor: xgf, xgAg: xga, gf: gf, ga: ga})
	}
	return out
}

func syncReadCSV(path string) ([][]string, map[string]int, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, false
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil || len(records) <= 1 {
		return nil, nil, false
	}
	return records, mapHeaders(records[0]), true
}
