package extraction

import "time"

type Report struct {
	ID               string          `json:"id"`
	SourceID         string          `json:"source_id"`
	SourceType       string          `json:"source_type"`
	SourceURL        string          `json:"source_url,omitempty"`
	AuthorHash       string          `json:"author_hash"`
	ConfidenceTier   int             `json:"confidence_tier"`
	DatePublished    *time.Time      `json:"date_published,omitempty"`
	ReportedSex      *string         `json:"reported_sex,omitempty"`
	ReportedAgeRange *string         `json:"reported_age_range,omitempty"`
	ReportedWeight   *WeightValue    `json:"reported_weight,omitempty"`
	TrainingStatus   *string         `json:"training_status,omitempty"`
	HealthConditions []string        `json:"health_conditions,omitempty"`
	PeptideExperience *string        `json:"peptide_experience,omitempty"`
	Compounds        []ReportCompound   `json:"compounds"`
	Benefits         []ReportBenefit    `json:"benefits,omitempty"`
	SideEffects      []ReportSideEffect `json:"side_effects,omitempty"`
	Biomarkers       []ReportBiomarker  `json:"biomarkers,omitempty"`
	ProtocolNotes    *string            `json:"protocol_notes,omitempty"`
	OverallSentiment string             `json:"overall_sentiment"`
	LLMConfidence    float64            `json:"llm_confidence"`
	ExtractionModel  string             `json:"extraction_model"`
	ExtractedAt      time.Time          `json:"extracted_at"`
}

type WeightValue struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type ReportCompound struct {
	NameRaw         string   `json:"name_raw"`
	NameNormalized  *string  `json:"name_normalized,omitempty"`
	Category        *string  `json:"category,omitempty"`
	Route           *string  `json:"route,omitempty"`
	DoseValue       *float64 `json:"dose_value,omitempty"`
	DoseUnit        *string  `json:"dose_unit,omitempty"`
	Frequency       *string  `json:"frequency,omitempty"`
	CycleLengthDays *int     `json:"cycle_length_days,omitempty"`
	IsPartOfStack   bool     `json:"is_part_of_stack"`
}

type ReportBenefit struct {
	Category    string  `json:"category"`
	Description *string `json:"description,omitempty"`
	Severity    *string `json:"severity,omitempty"`
	OnsetDays   *int    `json:"onset_days,omitempty"`
}

type ReportSideEffect struct {
	Category       string  `json:"category"`
	Description    *string `json:"description,omitempty"`
	Severity       *string `json:"severity,omitempty"`
	OnsetDays      *int    `json:"onset_days,omitempty"`
	Resolved       *bool   `json:"resolved,omitempty"`
	ResolutionDays *int    `json:"resolution_days,omitempty"`
}

type ReportBiomarker struct {
	MarkerName    string   `json:"marker_name"`
	ValueBefore   *float64 `json:"value_before,omitempty"`
	ValueAfter    *float64 `json:"value_after,omitempty"`
	Unit          *string  `json:"unit,omitempty"`
	TimeframeDays *int     `json:"timeframe_days,omitempty"`
}
