package normalize

import (
	"strings"
	"unicode"

	"github.com/teslashibe/verum-extract/compounds"
	"github.com/teslashibe/verum-extract/extraction"
)

type Normalizer struct {
	registry    *compounds.Registry
	autoRegister bool
}

type NormalizerOption func(*Normalizer)

func New(registry *compounds.Registry, opts ...NormalizerOption) *Normalizer {
	n := &Normalizer{registry: registry}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

func WithAutoRegister() NormalizerOption {
	return func(n *Normalizer) { n.autoRegister = true }
}

type Stats struct {
	TotalCompounds int                `json:"total_compounds"`
	Matched        int                `json:"matched"`
	Unmatched      int                `json:"unmatched"`
	AutoRegistered int                `json:"auto_registered"`
	MatchRate      float64            `json:"match_rate"`
	UnmatchedNames []UnmatchedCompound `json:"unmatched_names,omitempty"`
}

type UnmatchedCompound struct {
	NameRaw         string   `json:"name_raw"`
	OccurrenceCount int      `json:"occurrence_count"`
	ClosestMatch    string   `json:"closest_match,omitempty"`
	Distance        int      `json:"distance,omitempty"`
	SampleSourceIDs []string `json:"sample_source_ids,omitempty"`
}

func (n *Normalizer) NormalizeAll(reports []extraction.Report) Stats {
	unmatchedCounts := make(map[string]*UnmatchedCompound)

	stats := Stats{}
	for i := range reports {
		for j := range reports[i].Compounds {
			stats.TotalCompounds++
			c := &reports[i].Compounds[j]

			if compound := n.match(c.NameRaw); compound != nil {
				name := compound.Name
				cat := string(compound.Category)
				c.NameNormalized = &name
				c.Category = &cat
				stats.Matched++
			} else {
				key := cleanName(c.NameRaw)
				if n.autoRegister && key != "" {
					newComp := compounds.Compound{
						ID:          strings.ToLower(strings.ReplaceAll(key, "-", "_")),
						Name:        strings.ToLower(strings.ReplaceAll(key, "-", "_")),
						DisplayName: c.NameRaw,
						Aliases:     []string{c.NameRaw},
						Category:    compounds.CategoryOther,
					}
					n.registry.Add(newComp)
					name := newComp.Name
					cat := string(newComp.Category)
					c.NameNormalized = &name
					c.Category = &cat
					stats.Matched++
					stats.AutoRegistered++
				} else {
					stats.Unmatched++
					if entry, ok := unmatchedCounts[key]; ok {
						entry.OccurrenceCount++
						if len(entry.SampleSourceIDs) < 3 {
							entry.SampleSourceIDs = append(entry.SampleSourceIDs, reports[i].SourceID)
						}
					} else {
						closest, dist := n.closestMatch(c.NameRaw)
						unmatchedCounts[key] = &UnmatchedCompound{
							NameRaw:         c.NameRaw,
							OccurrenceCount: 1,
							ClosestMatch:    closest,
							Distance:        dist,
							SampleSourceIDs: []string{reports[i].SourceID},
						}
					}
				}
			}
		}
	}

	if stats.TotalCompounds > 0 {
		stats.MatchRate = float64(stats.Matched) / float64(stats.TotalCompounds)
	}
	for _, v := range unmatchedCounts {
		stats.UnmatchedNames = append(stats.UnmatchedNames, *v)
	}
	return stats
}

func (n *Normalizer) match(raw string) *compounds.Compound {
	if c, ok := n.registry.FindByAlias(raw); ok {
		return c
	}
	cleaned := cleanName(raw)
	if c, ok := n.registry.FindByAlias(cleaned); ok {
		return c
	}
	canonical := canonicalize(raw)
	if c, ok := n.registry.FindByAlias(canonical); ok {
		return c
	}
	if c := n.substringMatch(canonical); c != nil {
		return c
	}
	if c := n.fuzzyMatch(canonical); c != nil {
		return c
	}
	return nil
}

func (n *Normalizer) substringMatch(cleaned string) *compounds.Compound {
	lower := strings.ToLower(cleaned)
	if lower == "" || len(lower) < 3 {
		return nil
	}

	var best *compounds.Compound
	bestLen := 0

	for _, c := range n.registry.All() {
		candidates := append([]string{c.Name, c.DisplayName}, c.Aliases...)
		for _, alias := range candidates {
			al := strings.ToLower(alias)
			if len(al) < 3 {
				continue
			}
			if strings.Contains(lower, al) && len(al) > bestLen {
				bestLen = len(al)
				found := c
				best = &found
			}
		}
	}

	if best != nil && bestLen >= 3 && float64(bestLen)/float64(len(lower)) > 0.4 {
		return best
	}
	return nil
}

func (n *Normalizer) fuzzyMatch(raw string) *compounds.Compound {
	cleaned := strings.ToLower(cleanName(raw))
	bestDist := 3
	var best *compounds.Compound

	for _, c := range n.registry.All() {
		candidates := append([]string{c.DisplayName, c.Name}, c.Aliases...)
		for _, alias := range candidates {
			d := levenshtein(cleaned, strings.ToLower(alias))
			if d < bestDist {
				bestDist = d
				found := c
				best = &found
			}
		}
	}
	return best
}

func (n *Normalizer) closestMatch(raw string) (string, int) {
	cleaned := strings.ToLower(cleanName(raw))
	bestDist := 999
	bestName := ""

	for _, c := range n.registry.All() {
		candidates := append([]string{c.DisplayName}, c.Aliases...)
		for _, alias := range candidates {
			d := levenshtein(cleaned, strings.ToLower(alias))
			if d < bestDist {
				bestDist = d
				bestName = c.Name
			}
		}
	}
	return bestName, bestDist
}

func cleanName(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '-' || r == '–' || r == '—' || r == ' ':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

var stripWords = []string{
	"injection", "injectable", "injecting", "injected",
	"nasal", "intranasal", "nasal spray", "spray",
	"oral", "sublingual", "troches", "capsules", "pills", "tablets",
	"topical", "cream", "serum", "lotion", "rollon", "mist",
	"subcutaneous", "intramuscular", "intravenous", "subq",
	"blend", "combo", "mix", "stack", "combined", "compounded", "compound",
	"grey", "gray", "chinese", "pharmaceutical", "pharma", "pure", "full", "normal",
	"implied", "unspecified", "context", "from",
	"delayed", "delayed release", "rapid release",
	"cosmetic", "cosmetic grade", "grade",
	"liquid", "powder",
	"prescription",
	"mounjaro", "zepbound", "ozempic", "wegovy",
}

var abbreviations = map[string]string{
	"bpc":     "BPC-157",
	"bpc157":  "BPC-157",
	"bpc 157": "BPC-157",
	"tb":      "TB-500",
	"tb4":     "TB-500",
	"tb500":   "TB-500",
	"tb 500":  "TB-500",
	"thymosin beta-4":  "TB-500",
	"thymosin beta 4":  "TB-500",
	"ipa":     "Ipamorelin",
	"cjc":     "CJC-1295",
	"cjc1295": "CJC-1295",
	"tesa":    "Tesamorelin",
	"sema":    "Semaglutide",
	"tirz":    "Tirzepatide",
	"reta":    "Retatrutide",
	"mt2":     "Melanotan II",
	"mt-2":    "Melanotan II",
	"mt 2":    "Melanotan II",
	"mt1":     "Melanotan I",
	"mt-1":    "Melanotan I",
	"ghk":     "GHK-Cu",
	"ghkcu":   "GHK-Cu",
	"hgh frag":      "HGH Fragment 176-191",
	"hgh fragment":  "HGH Fragment 176-191",
	"na selank":     "Selank",
	"na-selank":     "Selank",
	"n-acetyl-selank":    "Selank",
	"n-acetyl selank":    "Selank",
	"n acetyl selank":    "Selank",
	"na semax":      "Semax",
	"na-semax":      "Semax",
	"n-acetyl-semax":     "Semax",
	"n-acetyl semax":     "Semax",
	"n acetyl semax":     "Semax",
	"na semax amidate":   "Semax",
	"n-acetyl amidate semax": "Semax",
	"semax amidate":      "Semax",
	"hgh secretagogues":  "HGH",
	"cjc no dac":    "CJC-1295",
	"cjc-1295 dac":  "CJC-1295 DAC",
	"cjc with dac":  "CJC-1295 DAC",
	"cjc w dac":     "CJC-1295 DAC",
	"gonadorelin":   "Gonadorelin",
	"nad":           "NAD+",
	"wolverine":     "BPC-157",
}

func canonicalize(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))

	if mapped, ok := abbreviations[s]; ok {
		return mapped
	}

	for prefix, mapped := range abbreviations {
		if strings.HasPrefix(s, prefix+" ") || strings.HasPrefix(s, prefix+"/") || strings.HasPrefix(s, prefix+"(") {
			return mapped
		}
	}

	words := strings.Fields(s)
	var kept []string
	for _, w := range words {
		strip := false
		for _, sw := range stripWords {
			if w == sw || w == sw+"s" || w == sw+"ed" || w == sw+"ing" {
				strip = true
				break
			}
		}
		if !strip {
			kept = append(kept, w)
		}
	}
	s = strings.Join(kept, " ")

	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.Trim(s, " -/()")

	if mapped, ok := abbreviations[s]; ok {
		return mapped
	}

	return s
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
