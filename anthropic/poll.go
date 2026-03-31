package anthropic

import (
	"context"
	"fmt"
	"time"
)

type PollOptions struct {
	Interval time.Duration
	OnStatus func(Batch)
}

func (c *Client) WaitForBatch(ctx context.Context, batchID string, opts PollOptions) (*Batch, error) {
	interval := opts.Interval
	if interval == 0 {
		interval = 30 * time.Second
	}

	for {
		batch, err := c.GetBatch(ctx, batchID)
		if err != nil {
			return nil, fmt.Errorf("poll batch %s: %w", batchID, err)
		}

		if opts.OnStatus != nil {
			opts.OnStatus(*batch)
		}

		switch batch.Status {
		case "ended":
			return batch, nil
		case "canceled", "expired":
			return batch, fmt.Errorf("batch %s ended with status: %s", batchID, batch.Status)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}
