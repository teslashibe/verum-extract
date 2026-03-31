package extraction

import (
	"fmt"
	"strings"
)

const systemPrompt = `You are a peptide research data extractor. Your job is to extract structured report records from community posts about peptide usage.

RULES:
1. Extract one report per distinct person/experience. The original poster (OP) is one report. Each commenter who shares their own protocol details is a separate report.
2. Only extract reports where someone describes their own peptide usage. Skip generic questions, vendor discussions, or pure theory.
3. Use null for any field you cannot determine from the text. NEVER fabricate or infer values not explicitly stated.
4. Self-rate your extraction confidence from 0.0 to 1.0 based on how much detail was available.
5. Assign a confidence_tier:
   - 3: User report includes before/after bloodwork, labs, or body scans
   - 4: Detailed protocol with specific doses, timelines, and subjective outcomes
   - 5: Vague mention, casual comment, no specific protocol details

OUTPUT FORMAT:
Respond with ONLY a JSON array of report objects. No explanation, no markdown fences, just the JSON array.
If no extractable reports exist in the post, respond with an empty array: []

REPORT SCHEMA:
[
  {
    "author": "string (reddit username, will be hashed later)",
    "confidence_tier": 3|4|5,
    "reported_sex": "male"|"female"|null,
    "reported_age_range": "18-24"|"25-29"|"30-39"|"40-49"|"50-59"|"60+"|null,
    "reported_weight": {"value": number, "unit": "lbs"|"kg"} or null,
    "training_status": "string or null",
    "health_conditions": ["string"] or null,
    "peptide_experience": "naive"|"experienced"|null,
    "compounds": [
      {
        "name_raw": "string (exactly as mentioned)",
        "route": "subcutaneous"|"intramuscular"|"oral"|"nasal"|"topical"|"intravenous"|null,
        "dose_value": number or null,
        "dose_unit": "mcg"|"mg"|"IU"|null,
        "frequency": "string (e.g. '2x daily', 'every other day')" or null,
        "cycle_length_days": number or null,
        "is_part_of_stack": boolean
      }
    ],
    "benefits": [
      {
        "category": "injury_healing"|"fat_loss"|"muscle_gain"|"sleep"|"energy"|"libido"|"cognitive"|"skin"|"hair"|"mood"|"anti_aging"|"appetite_suppression"|"immune"|"gut_health"|"joint_pain"|"recovery"|"other",
        "description": "string" or null,
        "severity": "significant"|"moderate"|"mild"|null,
        "onset_days": number or null
      }
    ],
    "side_effects": [
      {
        "category": "nausea"|"fatigue"|"injection_site"|"headache"|"water_retention"|"blood_pressure"|"mood_change"|"libido_change"|"appetite_change"|"sleep_disruption"|"flushing"|"dizziness"|"numbness_tingling"|"other",
        "description": "string" or null,
        "severity": "significant"|"moderate"|"mild"|null,
        "onset_days": number or null,
        "resolved": boolean or null,
        "resolution_days": number or null
      }
    ],
    "biomarkers": [
      {
        "marker_name": "string (e.g. IGF-1, testosterone, A1C)",
        "value_before": number or null,
        "value_after": number or null,
        "unit": "string" or null,
        "timeframe_days": number or null
      }
    ],
    "protocol_notes": "string (any additional context)" or null,
    "overall_sentiment": "positive"|"mixed"|"negative"|"neutral",
    "llm_confidence": 0.0-1.0
  }
]`

func BuildUserPrompt(input ExtractionInput) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Extract structured peptide reports from this Reddit post.\n\n"))
	b.WriteString(fmt.Sprintf("Subreddit: r/%s\n", input.SourceMeta["subreddit"]))
	b.WriteString(fmt.Sprintf("Title: %s\n", input.Title))
	b.WriteString(fmt.Sprintf("Score: %v | Comments: %v | Date: %s\n\n",
		input.SourceMeta["score"],
		input.SourceMeta["num_comments"],
		input.PublishedAt.Format("2006-01-02"),
	))

	if input.Body != "" {
		b.WriteString("Post body:\n")
		body := input.Body
		if len(body) > 8000 {
			body = body[:8000] + "\n[...truncated]"
		}
		b.WriteString(body)
		b.WriteString("\n\n")
	}

	if len(input.Comments) > 0 {
		b.WriteString("Comments:\n")
		for i, c := range input.Comments {
			indent := strings.Repeat("  ", c.Depth)
			submitter := ""
			if c.IsSubmitter {
				submitter = " [OP]"
			}
			b.WriteString(fmt.Sprintf("%s[%d] %s%s (score: %d):\n%s%s\n\n",
				indent, i+1, c.Author, submitter, c.Score,
				indent, c.Body,
			))
			if i >= 49 {
				b.WriteString(fmt.Sprintf("[...%d more comments not shown]\n", len(input.Comments)-50))
				break
			}
		}
	}

	return b.String()
}
