package aggregate

import "time"

type CompoundProfile struct {
	CompoundID         string              `json:"compound_id"`
	DisplayName        string              `json:"display_name"`
	Category           string              `json:"category"`
	TotalReports       int                 `json:"total_reports"`
	ReportsByTier      map[int]int         `json:"reports_by_tier"`
	TopBenefits        []RankedItem        `json:"top_benefits"`
	TopSideEffects     []RankedSideEffect  `json:"top_side_effects"`
	CommonDoses        []DoseProtocol      `json:"common_doses"`
	CommonStacks       []StackEntry        `json:"common_stacks"`
	AvgCycleLengthDays *float64            `json:"avg_cycle_length_days,omitempty"`
	AvgSentimentScore  *float64            `json:"avg_sentiment_score,omitempty"`
	SentimentBreakdown SentimentBreakdown  `json:"sentiment_breakdown"`
	LimitedData        bool                `json:"limited_data"`
	GeneratedAt        time.Time           `json:"generated_at"`
}

type RankedItem struct {
	Category string  `json:"category"`
	Count    int     `json:"count"`
	Pct      float64 `json:"pct"`
}

type RankedSideEffect struct {
	Category    string  `json:"category"`
	Count       int     `json:"count"`
	Pct         float64 `json:"pct"`
	AvgSeverity float64 `json:"avg_severity"`
}

type DoseProtocol struct {
	Route     string  `json:"route"`
	Dose      float64 `json:"dose"`
	Unit      string  `json:"unit"`
	Frequency string  `json:"frequency"`
	Count     int     `json:"count"`
	Pct       float64 `json:"pct"`
}

type StackEntry struct {
	CompoundID  string `json:"compound_id"`
	DisplayName string `json:"display_name"`
	ReportCount int    `json:"report_count"`
}

type SentimentBreakdown struct {
	Positive int `json:"positive"`
	Mixed    int `json:"mixed"`
	Negative int `json:"negative"`
	Neutral  int `json:"neutral"`
}

type AggregationResult struct {
	Profiles          []CompoundProfile `json:"profiles"`
	TotalReports      int               `json:"total_reports"`
	CompoundsFound    int               `json:"compounds_found"`
	CompoundsProfiled int               `json:"compounds_profiled"`
}
