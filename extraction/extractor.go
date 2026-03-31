package extraction

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"time"

	"github.com/teslashibe/verum-extract/anthropic"
)

type Extractor struct {
	client    *anthropic.Client
	salt      string
	batchSize int
}

type ExtractorOption func(*Extractor)

func NewExtractor(client *anthropic.Client, opts ...ExtractorOption) *Extractor {
	e := &Extractor{
		client:    client,
		salt:      "verum-extract-default-salt",
		batchSize: 1000,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func WithAuthorSalt(salt string) ExtractorOption { return func(e *Extractor) { e.salt = salt } }
func WithBatchSize(n int) ExtractorOption        { return func(e *Extractor) { e.batchSize = n } }

type ExtractionResult struct {
	Reports   []Report
	Errors    []ExtractionError
	BatchIDs  []string
	Succeeded int
	Failed    int
	Duration  time.Duration
}

type ExtractionError struct {
	SourceID string `json:"source_id"`
	Error    string `json:"error"`
}

func (e *Extractor) ExtractBatch(ctx context.Context, inputs []ExtractionInput, onProgress func(string)) (*ExtractionResult, error) {
	start := time.Now()
	result := &ExtractionResult{}

	chunks := anthropic.ChunkRequests(e.buildRequests(inputs), e.batchSize)
	inputMap := make(map[string]ExtractionInput, len(inputs))
	for _, input := range inputs {
		inputMap[input.SourceID] = input
	}

	for i, chunk := range chunks {
		if onProgress != nil {
			onProgress(fmt.Sprintf("Submitting batch %d/%d (%d posts)...", i+1, len(chunks), len(chunk)))
		}

		batch, err := e.client.CreateBatch(ctx, chunk)
		if err != nil {
			return result, fmt.Errorf("create batch %d: %w", i+1, err)
		}
		result.BatchIDs = append(result.BatchIDs, batch.ID)

		if onProgress != nil {
			onProgress(fmt.Sprintf("Waiting for batch %s...", batch.ID))
		}

		batch, err = e.client.WaitForBatch(ctx, batch.ID, anthropic.PollOptions{
			Interval: 30 * time.Second,
			OnStatus: func(b anthropic.Batch) {
				total := b.RequestCounts.Succeeded + b.RequestCounts.Errored + b.RequestCounts.Canceled + b.RequestCounts.Expired
				if onProgress != nil {
					onProgress(fmt.Sprintf("  Batch %s: %d/%d processed", b.ID, total, total+b.RequestCounts.Processing))
				}
			},
		})
		if err != nil {
			return result, fmt.Errorf("wait for batch %s: %w", batch.ID, err)
		}

		results, err := e.client.GetBatchResults(ctx, batch.ID)
		if err != nil {
			return result, fmt.Errorf("get results for batch %s: %w", batch.ID, err)
		}

		for _, br := range results {
			if br.Result.Type != "succeeded" || br.Result.Message == nil {
				result.Failed++
				errMsg := "unknown error"
				if br.Result.Error != nil {
					errMsg = br.Result.Error.Message
				}
				result.Errors = append(result.Errors, ExtractionError{
					SourceID: br.CustomID,
					Error:    errMsg,
				})
				continue
			}

			text := ""
			for _, block := range br.Result.Message.Content {
				if block.Type == "text" {
					text += block.Text
				}
			}

			input, ok := inputMap[br.CustomID]
			if !ok {
				log.Printf("warning: result for unknown source ID: %s", br.CustomID)
				continue
			}

			reports, err := ParseResponse(text, input, e.client.Model(), e.hashAuthor)
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, ExtractionError{
					SourceID: br.CustomID,
					Error:    fmt.Sprintf("parse error: %v", err),
				})
				continue
			}

			result.Reports = append(result.Reports, reports...)
			result.Succeeded++
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

func (e *Extractor) buildRequests(inputs []ExtractionInput) []anthropic.BatchRequest {
	reqs := make([]anthropic.BatchRequest, len(inputs))
	for i, input := range inputs {
		reqs[i] = anthropic.BatchRequest{
			CustomID:  input.SourceID,
			System:    systemPrompt,
			Messages:  []anthropic.Message{{Role: "user", Content: BuildUserPrompt(input)}},
			MaxTokens: 4096,
		}
	}
	return reqs
}

func (e *Extractor) hashAuthor(author string) string {
	h := sha256.Sum256([]byte(e.salt + ":" + author))
	return fmt.Sprintf("%x", h[:])
}
