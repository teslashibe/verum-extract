package compounds

type Category string

const (
	CategoryHealingRecovery  Category = "healing_recovery"
	CategoryGHSecretagogue   Category = "gh_secretagogue"
	CategoryBodyComposition  Category = "body_composition"
	CategoryCognitive        Category = "cognitive"
	CategoryAntiAging        Category = "anti_aging"
	CategoryImmune           Category = "immune"
	CategorySexualHealth     Category = "sexual_health"
	CategoryMuscle           Category = "muscle_performance"
	CategoryGutHealth        Category = "gut_health"
	CategoryPainInflammation Category = "pain_inflammation"
	CategorySkinCosmetic     Category = "skin_cosmetic"
	CategoryTanning          Category = "tanning"
	CategoryMetabolic        Category = "metabolic"
	CategoryMitochondrial    Category = "mitochondrial"
	CategoryHormonal         Category = "hormonal"
	CategoryEmerging         Category = "emerging_research"
	CategoryOther            Category = "other"
)

type Compound struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	Aliases      []string `json:"aliases,omitempty"`
	Category     Category `json:"category"`
	Description  string   `json:"description,omitempty"`
	Mechanism    string   `json:"mechanism,omitempty"`
	CommonRoutes []string `json:"common_routes,omitempty"`
	DoseUnit     string   `json:"dose_unit,omitempty"`
}
