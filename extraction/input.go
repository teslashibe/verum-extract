package extraction

import "time"

type ExtractionInput struct {
	SourceID   string         `json:"source_id"`
	SourceType string         `json:"source_type"`
	SourceURL  string         `json:"source_url,omitempty"`
	SourceMeta map[string]any `json:"source_meta,omitempty"`
	Title      string         `json:"title"`
	Body       string         `json:"body"`
	Comments   []CommentInput `json:"comments,omitempty"`
	PublishedAt time.Time     `json:"published_at"`
}

type CommentInput struct {
	Author      string `json:"author"`
	Body        string `json:"body"`
	Score       int    `json:"score"`
	Depth       int    `json:"depth"`
	IsSubmitter bool   `json:"is_submitter"`
}
