package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultBaseURL   = "https://api.anthropic.com"
	defaultModel     = "claude-haiku-4-5-20251001"
	defaultMaxTokens = 4096
	apiVersion       = "2023-06-01"
	betaHeader       = "message-batches-2024-09-24"
)

type Client struct {
	apiKey     string
	baseURL    string
	model      string
	maxTokens  int
	httpClient *http.Client
}

type Option func(*Client)

func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:    apiKey,
		baseURL:   defaultBaseURL,
		model:     defaultModel,
		maxTokens: defaultMaxTokens,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func WithModel(model string) Option         { return func(c *Client) { c.model = model } }
func WithBaseURL(url string) Option          { return func(c *Client) { c.baseURL = url } }
func WithHTTPClient(hc *http.Client) Option  { return func(c *Client) { c.httpClient = hc } }
func WithMaxTokens(n int) Option             { return func(c *Client) { c.maxTokens = n } }

func (c *Client) Model() string { return c.model }

func (c *Client) CreateBatch(ctx context.Context, requests []BatchRequest) (*Batch, error) {
	items := make([]batchRequestItem, len(requests))
	for i, r := range requests {
		maxTok := r.MaxTokens
		if maxTok == 0 {
			maxTok = c.maxTokens
		}
		msgs := make([]messagePayload, len(r.Messages))
		for j, m := range r.Messages {
			msgs[j] = messagePayload{Role: m.Role, Content: m.Content}
		}
		items[i] = batchRequestItem{
			CustomID: r.CustomID,
			Params: batchRequestParams{
				Model:     c.model,
				MaxTokens: maxTok,
				System:    r.System,
				Messages:  msgs,
			},
		}
	}

	body := batchCreateRequest{Requests: items}
	var batch Batch
	if err := c.do(ctx, http.MethodPost, "/v1/messages/batches", body, &batch); err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}
	return &batch, nil
}

func (c *Client) GetBatch(ctx context.Context, batchID string) (*Batch, error) {
	var batch Batch
	if err := c.do(ctx, http.MethodGet, "/v1/messages/batches/"+batchID, nil, &batch); err != nil {
		return nil, fmt.Errorf("get batch: %w", err)
	}
	return &batch, nil
}

func (c *Client) GetBatchResults(ctx context.Context, batchID string) ([]BatchResult, error) {
	path := "/v1/messages/batches/" + batchID + "/results"
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("batch results request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var results []BatchResult
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r BatchResult
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		results = append(results, r)
	}
	if err := scanner.Err(); err != nil {
		return results, fmt.Errorf("scanning results: %w", err)
	}
	return results, nil
}

func (c *Client) CancelBatch(ctx context.Context, batchID string) error {
	return c.do(ctx, http.MethodPost, "/v1/messages/batches/"+batchID+"/cancel", nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.readError(resp)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)
	req.Header.Set("anthropic-beta", betaHeader)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *Client) readError(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(b))
}

func ChunkRequests(requests []BatchRequest, chunkSize int) [][]BatchRequest {
	if chunkSize <= 0 {
		chunkSize = 10000
	}
	var chunks [][]BatchRequest
	for i := 0; i < len(requests); i += chunkSize {
		end := i + chunkSize
		if end > len(requests) {
			end = len(requests)
		}
		chunks = append(chunks, requests[i:end])
	}
	return chunks
}
