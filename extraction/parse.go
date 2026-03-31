package extraction

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/teslashibe/peptidebase/internal/uid"
)

type rawReport struct {
	Author            string          `json:"author"`
	ConfidenceTier    int             `json:"confidence_tier"`
	ReportedSex       *string         `json:"reported_sex"`
	ReportedAgeRange  *string         `json:"reported_age_range"`
	ReportedWeight    *WeightValue    `json:"reported_weight"`
	TrainingStatus    *string         `json:"training_status"`
	HealthConditions  []string        `json:"health_conditions"`
	PeptideExperience *string         `json:"peptide_experience"`
	Compounds         []ReportCompound   `json:"compounds"`
	Benefits          []ReportBenefit    `json:"benefits"`
	SideEffects       []ReportSideEffect `json:"side_effects"`
	Biomarkers        []ReportBiomarker  `json:"biomarkers"`
	ProtocolNotes     *string            `json:"protocol_notes"`
	OverallSentiment  string             `json:"overall_sentiment"`
	LLMConfidence     float64            `json:"llm_confidence"`
}

func ParseResponse(response string, input ExtractionInput, model string, hashFn func(string) string) ([]Report, error) {
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON array found in response")
	}

	var raws []rawReport
	if err := json.Unmarshal([]byte(jsonStr), &raws); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	now := time.Now().UTC()
	var reports []Report

	for _, raw := range raws {
		if len(raw.Compounds) == 0 {
			continue
		}

		r := Report{
			ID:                uid.New(),
			SourceID:          input.SourceID,
			SourceType:        input.SourceType,
			SourceURL:         input.SourceURL,
			AuthorHash:        hashFn(raw.Author),
			ConfidenceTier:    clampTier(raw.ConfidenceTier),
			DatePublished:     &input.PublishedAt,
			ReportedSex:       validateEnum(raw.ReportedSex, validSex),
			ReportedAgeRange:  validateEnum(raw.ReportedAgeRange, validAgeRange),
			ReportedWeight:    raw.ReportedWeight,
			TrainingStatus:    raw.TrainingStatus,
			HealthConditions:  raw.HealthConditions,
			PeptideExperience: validateEnum(raw.PeptideExperience, validExperience),
			Compounds:         validateCompounds(raw.Compounds),
			Benefits:          validateBenefits(raw.Benefits),
			SideEffects:       validateSideEffects(raw.SideEffects),
			Biomarkers:        raw.Biomarkers,
			ProtocolNotes:     raw.ProtocolNotes,
			OverallSentiment:  validateSentiment(raw.OverallSentiment),
			LLMConfidence:     clampFloat(raw.LLMConfidence, 0, 1),
			ExtractionModel:   model,
			ExtractedAt:       now,
		}
		reports = append(reports, r)
	}

	return reports, nil
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) > 1 {
			s = lines[1]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}

	start := strings.Index(s, "[")
	if start < 0 {
		return ""
	}
	end := strings.LastIndex(s, "]")
	if end < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

var (
	validSex        = map[string]bool{"male": true, "female": true}
	validAgeRange   = map[string]bool{"18-24": true, "25-29": true, "30-39": true, "40-49": true, "50-59": true, "60+": true}
	validExperience = map[string]bool{"naive": true, "experienced": true}
	validSentiment  = map[string]bool{"positive": true, "mixed": true, "negative": true, "neutral": true}
	validSeverity   = map[string]bool{"significant": true, "moderate": true, "mild": true}
	validRoute      = map[string]bool{"subcutaneous": true, "intramuscular": true, "oral": true, "nasal": true, "topical": true, "intravenous": true}
)

func validateEnum(val *string, valid map[string]bool) *string {
	if val == nil {
		return nil
	}
	lower := strings.ToLower(strings.TrimSpace(*val))
	if valid[lower] {
		return &lower
	}
	return nil
}

func validateSentiment(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	if validSentiment[lower] {
		return lower
	}
	return "neutral"
}

func validateCompounds(comps []ReportCompound) []ReportCompound {
	for i := range comps {
		comps[i].Route = validateEnum(comps[i].Route, validRoute)
	}
	return comps
}

func validateBenefits(benefits []ReportBenefit) []ReportBenefit {
	for i := range benefits {
		benefits[i].Severity = validateEnum(benefits[i].Severity, validSeverity)
	}
	return benefits
}

func validateSideEffects(effects []ReportSideEffect) []ReportSideEffect {
	for i := range effects {
		effects[i].Severity = validateEnum(effects[i].Severity, validSeverity)
	}
	return effects
}

func clampTier(t int) int {
	if t < 3 {
		return 5
	}
	if t > 5 {
		return 5
	}
	return t
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
