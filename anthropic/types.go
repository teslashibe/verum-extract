package anthropic

import "time"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type BatchRequest struct {
	CustomID  string
	System    string
	Messages  []Message
	MaxTokens int
}

type Batch struct {
	ID            string        `json:"id"`
	Type          string        `json:"type"`
	Status        string        `json:"processing_status"`
	RequestCounts RequestCounts `json:"request_counts"`
	CreatedAt     time.Time     `json:"created_at"`
	EndedAt       *time.Time    `json:"ended_at,omitempty"`
	ResultsURL    *string       `json:"results_url,omitempty"`
}

type RequestCounts struct {
	Processing int `json:"processing"`
	Succeeded  int `json:"succeeded"`
	Errored    int `json:"errored"`
	Canceled   int `json:"canceled"`
	Expired    int `json:"expired"`
}

type BatchResult struct {
	CustomID string           `json:"custom_id"`
	Result   BatchResultInner `json:"result"`
}

type BatchResultInner struct {
	Type    string           `json:"type"`
	Message *MessageResponse `json:"message,omitempty"`
	Error   *APIError        `json:"error,omitempty"`
}

type MessageResponse struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
	Model   string         `json:"model"`
	Usage   Usage          `json:"usage"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type APIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return e.Type + ": " + e.Message
}

// API request/response shapes for batch creation.

type batchCreateRequest struct {
	Requests []batchRequestItem `json:"requests"`
}

type batchRequestItem struct {
	CustomID string             `json:"custom_id"`
	Params   batchRequestParams `json:"params"`
}

type batchRequestParams struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    string           `json:"system,omitempty"`
	Messages  []messagePayload `json:"messages"`
}

type messagePayload struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
