package extraction

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"sync"
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

	// Submit all batches upfront
	type submittedBatch struct {
		id    string
		index int
		size  int
	}
	var batches []submittedBatch

	for i, chunk := range chunks {
		if onProgress != nil {
			onProgress(fmt.Sprintf("Submitting batch %d/%d (%d posts)...", i+1, len(chunks), len(chunk)))
		}
		batch, err := e.client.CreateBatch(ctx, chunk)
		if err != nil {
			return result, fmt.Errorf("create batch %d: %w", i+1, err)
		}
		batches = append(batches, submittedBatch{id: batch.ID, index: i + 1, size: len(chunk)})
		result.BatchIDs = append(result.BatchIDs, batch.ID)
	}

	if onProgress != nil {
		onProgress(fmt.Sprintf("All %d batches submitted — waiting for completion...", len(batches)))
	}

	// Poll all batches concurrently
	type batchOutput struct {
		results []anthropic.BatchResult
		err     error
		index   int
	}
	outputs := make([]batchOutput, len(batches))
	var wg sync.WaitGroup

	for i, sb := range batches {
		wg.Add(1)
		go func(idx int, b submittedBatch) {
			defer wg.Done()
			_, err := e.client.WaitForBatch(ctx, b.id, anthropic.PollOptions{
				Interval: 30 * time.Second,
				OnStatus: func(batch anthropic.Batch) {
					done := batch.RequestCounts.Succeeded + batch.RequestCounts.Errored + batch.RequestCounts.Canceled + batch.RequestCounts.Expired
					total := done + batch.RequestCounts.Processing
					if onProgress != nil {
						onProgress(fmt.Sprintf("  Batch %d/%d (%s): %d/%d processed", b.index, len(batches), b.id[:12], done, total))
					}
				},
			})
			if err != nil {
				outputs[idx] = batchOutput{err: fmt.Errorf("wait for batch %s: %w", b.id, err), index: b.index}
				return
			}
			results, err := e.client.GetBatchResults(ctx, b.id)
			if err != nil {
				outputs[idx] = batchOutput{err: fmt.Errorf("get results for batch %s: %w", b.id, err), index: b.index}
				return
			}
			outputs[idx] = batchOutput{results: results, index: b.index}
		}(i, sb)
	}

	wg.Wait()

	// Collect results from all batches
	for _, out := range outputs {
		if out.err != nil {
			return result, out.err
		}
		for _, br := range out.results {
			if br.Result.Type != "succeeded" || br.Result.Message == nil {
				result.Failed++
				errMsg := fmt.Sprintf("result_type=%s", br.Result.Type)
				if br.Result.Error != nil {
					errMsg += fmt.Sprintf(" error_type=%s msg=%s", br.Result.Error.Type, br.Result.Error.Message)
				}
				if br.Result.Message == nil {
					errMsg += " (no message body)"
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
