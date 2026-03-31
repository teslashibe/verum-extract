package aggregate

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/teslashibe/verum-extract/compounds"
	"github.com/teslashibe/verum-extract/extraction"
)

func FromReports(reports []extraction.Report, registry *compounds.Registry) *AggregationResult {
	type compoundReports struct {
		reports []extraction.Report
		comps   []extraction.ReportCompound
	}

	grouped := make(map[string]*compoundReports)

	for _, r := range reports {
		for _, c := range r.Compounds {
			if c.NameNormalized == nil {
				continue
			}
			key := *c.NameNormalized
			if _, ok := grouped[key]; !ok {
				grouped[key] = &compoundReports{}
			}
			grouped[key].reports = append(grouped[key].reports, r)
			grouped[key].comps = append(grouped[key].comps, c)
		}
	}

	result := &AggregationResult{
		TotalReports:   len(reports),
		CompoundsFound: len(grouped),
	}

	for compoundName, data := range grouped {
		compound, ok := registry.FindByName(compoundName)
		if !ok {
			continue
		}

		profile := buildProfile(compound, data.reports, data.comps, compoundName)
		result.Profiles = append(result.Profiles, profile)
		result.CompoundsProfiled++
	}

	sort.Slice(result.Profiles, func(i, j int) bool {
		return result.Profiles[i].TotalReports > result.Profiles[j].TotalReports
	})

	return result
}

func buildProfile(compound *compounds.Compound, reports []extraction.Report, comps []extraction.ReportCompound, name string) CompoundProfile {
	seen := make(map[string]bool)
	var uniqueReports []extraction.Report
	for _, r := range reports {
		if !seen[r.ID] {
			seen[r.ID] = true
			uniqueReports = append(uniqueReports, r)
		}
	}

	total := len(uniqueReports)
	profile := CompoundProfile{
		CompoundID:    compound.ID,
		DisplayName:   compound.DisplayName,
		Category:      string(compound.Category),
		TotalReports:  total,
		ReportsByTier: make(map[int]int),
		LimitedData:   total < 5,
		GeneratedAt:   time.Now().UTC(),
	}

	for _, r := range uniqueReports {
		profile.ReportsByTier[r.ConfidenceTier]++
	}

	profile.SentimentBreakdown = countSentiment(uniqueReports)
	avgSent := avgSentimentScore(profile.SentimentBreakdown, total)
	profile.AvgSentimentScore = &avgSent

	detailedReports := filterTier5(uniqueReports)
	detailedTotal := len(detailedReports)
	if detailedTotal == 0 {
		detailedTotal = 1
	}

	profile.TopBenefits = rankBenefits(detailedReports, name, detailedTotal)
	profile.TopSideEffects = rankSideEffects(detailedReports, name, detailedTotal)
	profile.CommonDoses = rankDoses(comps, total)
	profile.CommonStacks = findStacks(uniqueReports, name, compound.ID)
	profile.AvgCycleLengthDays = avgCycleLength(comps)

	return profile
}

func filterTier5(reports []extraction.Report) []extraction.Report {
	var out []extraction.Report
	for _, r := range reports {
		if r.ConfidenceTier < 5 {
			out = append(out, r)
		}
	}
	return out
}

func rankBenefits(reports []extraction.Report, compoundName string, total int) []RankedItem {
	counts := make(map[string]int)
	for _, r := range reports {
		for _, b := range r.Benefits {
			counts[b.Category]++
		}
	}
	return toRankedItems(counts, total, 10)
}

func rankSideEffects(reports []extraction.Report, compoundName string, total int) []RankedSideEffect {
	type seData struct {
		count    int
		severity float64
		sevCount int
	}
	data := make(map[string]*seData)

	for _, r := range reports {
		for _, se := range r.SideEffects {
			if _, ok := data[se.Category]; !ok {
				data[se.Category] = &seData{}
			}
			d := data[se.Category]
			d.count++
			if se.Severity != nil {
				d.severity += severityScore(*se.Severity)
				d.sevCount++
			}
		}
	}

	var items []RankedSideEffect
	for cat, d := range data {
		avgSev := 0.0
		if d.sevCount > 0 {
			avgSev = math.Round(d.severity/float64(d.sevCount)*10) / 10
		}
		items = append(items, RankedSideEffect{
			Category:    cat,
			Count:       d.count,
			Pct:         math.Round(float64(d.count)/float64(total)*1000) / 10,
			AvgSeverity: avgSev,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	if len(items) > 10 {
		items = items[:10]
	}
	return items
}

func rankDoses(comps []extraction.ReportCompound, total int) []DoseProtocol {
	type doseKey struct {
		route, unit, freq string
		dose              float64
	}
	counts := make(map[doseKey]int)

	for _, c := range comps {
		if c.Route == nil || c.DoseValue == nil || c.DoseUnit == nil {
			continue
		}
		freq := ""
		if c.Frequency != nil {
			freq = *c.Frequency
		}
		key := doseKey{
			route: *c.Route,
			dose:  *c.DoseValue,
			unit:  *c.DoseUnit,
			freq:  strings.ToLower(freq),
		}
		counts[key]++
	}

	var items []DoseProtocol
	for k, count := range counts {
		items = append(items, DoseProtocol{
			Route:     k.route,
			Dose:      k.dose,
			Unit:      k.unit,
			Frequency: k.freq,
			Count:     count,
			Pct:       math.Round(float64(count)/float64(total)*1000) / 10,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	if len(items) > 5 {
		items = items[:5]
	}
	return items
}

func findStacks(reports []extraction.Report, targetName, targetID string) []StackEntry {
	counts := make(map[string]int)
	names := make(map[string]string)

	for _, r := range reports {
		has := false
		for _, c := range r.Compounds {
			if c.NameNormalized != nil && *c.NameNormalized == targetName {
				has = true
				break
			}
		}
		if !has {
			continue
		}
		for _, c := range r.Compounds {
			if c.NameNormalized == nil || *c.NameNormalized == targetName {
				continue
			}
			counts[*c.NameNormalized]++
			names[*c.NameNormalized] = c.NameRaw
		}
	}

	var items []StackEntry
	for id, count := range counts {
		items = append(items, StackEntry{
			CompoundID:  id,
			DisplayName: names[id],
			ReportCount: count,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ReportCount > items[j].ReportCount })
	if len(items) > 5 {
		items = items[:5]
	}
	return items
}

func avgCycleLength(comps []extraction.ReportCompound) *float64 {
	var sum, count float64
	for _, c := range comps {
		if c.CycleLengthDays != nil && *c.CycleLengthDays > 0 {
			sum += float64(*c.CycleLengthDays)
			count++
		}
	}
	if count == 0 {
		return nil
	}
	avg := math.Round(sum/count*10) / 10
	return &avg
}

func countSentiment(reports []extraction.Report) SentimentBreakdown {
	var sb SentimentBreakdown
	for _, r := range reports {
		switch r.OverallSentiment {
		case "positive":
			sb.Positive++
		case "mixed":
			sb.Mixed++
		case "negative":
			sb.Negative++
		case "neutral":
			sb.Neutral++
		}
	}
	return sb
}

func avgSentimentScore(sb SentimentBreakdown, total int) float64 {
	if total == 0 {
		return 0
	}
	score := float64(sb.Positive)*1.0 + float64(sb.Mixed)*0.5 + float64(sb.Neutral)*0.25
	return math.Round(score/float64(total)*1000) / 1000
}

func severityScore(s string) float64 {
	switch s {
	case "significant":
		return 3
	case "moderate":
		return 2
	case "mild":
		return 1
	default:
		return 0
	}
}

func toRankedItems(counts map[string]int, total, limit int) []RankedItem {
	var items []RankedItem
	for cat, count := range counts {
		items = append(items, RankedItem{
			Category: cat,
			Count:    count,
			Pct:      math.Round(float64(count)/float64(total)*1000) / 10,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func WriteProfiles(profiles []CompoundProfile, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, p := range profiles {
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", p.CompoundID, err)
		}
		path := filepath.Join(dir, p.CompoundID+".json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func ReadReports(dir string) ([]extraction.Report, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var reports []extraction.Report
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var r extraction.Report
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("parse %s: %w", entry.Name(), err)
		}
		reports = append(reports, r)
	}
	return reports, nil
}
