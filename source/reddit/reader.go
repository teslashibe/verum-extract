package reddit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/teslashibe/verum-extract/extraction"
)

type Reader struct {
	maxCommentDepth int
	maxComments     int
	minCommentScore int
}

type Option func(*Reader)

func NewReader(opts ...Option) *Reader {
	r := &Reader{
		maxCommentDepth: 3,
		maxComments:     50,
		minCommentScore: -5,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func WithMaxCommentDepth(d int) Option { return func(r *Reader) { r.maxCommentDepth = d } }
func WithMaxComments(n int) Option     { return func(r *Reader) { r.maxComments = n } }
func WithMinCommentScore(s int) Option { return func(r *Reader) { r.minCommentScore = s } }

type ReadStats struct {
	TotalLines  int            `json:"total_lines"`
	Parsed      int            `json:"parsed"`
	Skipped     int            `json:"skipped"`
	SkipReasons map[string]int `json:"skip_reasons"`
	Errors      int            `json:"errors"`
}

func (r *Reader) ReadFile(path string) ([]extraction.ExtractionInput, ReadStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ReadStats{}, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	return r.ReadLines(f)
}

func (r *Reader) ReadLines(reader io.Reader) ([]extraction.ExtractionInput, ReadStats, error) {
	stats := ReadStats{SkipReasons: make(map[string]int)}
	var inputs []extraction.ExtractionInput

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 20*1024*1024)

	for scanner.Scan() {
		stats.TotalLines++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var post Post
		if err := json.Unmarshal(line, &post); err != nil {
			stats.Errors++
			continue
		}

		if reason := r.shouldSkip(post); reason != "" {
			stats.Skipped++
			stats.SkipReasons[reason]++
			continue
		}

		input := r.toInput(post)
		inputs = append(inputs, input)
		stats.Parsed++
	}

	if err := scanner.Err(); err != nil {
		return inputs, stats, fmt.Errorf("scanner error: %w", err)
	}
	return inputs, stats, nil
}

func (r *Reader) shouldSkip(p Post) string {
	if p.Stickied {
		return "stickied"
	}
	if p.Score < -10 {
		return "low_score"
	}
	text := strings.TrimSpace(p.SelfText)
	if text == "[removed]" || text == "[deleted]" {
		return "removed"
	}
	if !p.IsSelf && text == "" {
		return "link_only"
	}
	return ""
}

func (r *Reader) toInput(p Post) extraction.ExtractionInput {
	comments := r.flattenComments(p.Comments)

	sort.Slice(comments, func(i, j int) bool {
		return comments[i].Score > comments[j].Score
	})
	if len(comments) > r.maxComments {
		comments = comments[:r.maxComments]
	}

	sourceURL := "https://www.reddit.com" + p.Permalink
	meta := map[string]any{
		"subreddit":   p.Subreddit,
		"score":       p.Score,
		"upvote_ratio": p.UpvoteRatio,
		"num_comments": p.NumComments,
		"flair":        p.LinkFlairText,
	}

	return extraction.ExtractionInput{
		SourceID:    p.ID,
		SourceType:  "reddit",
		SourceURL:   sourceURL,
		SourceMeta:  meta,
		Title:       p.Title,
		Body:        p.SelfText,
		Comments:    comments,
		PublishedAt: p.CreatedUTC,
	}
}

func (r *Reader) flattenComments(comments []Comment) []extraction.CommentInput {
	var flat []extraction.CommentInput
	for _, c := range comments {
		r.flattenComment(c, &flat)
	}
	return flat
}

func (r *Reader) flattenComment(c Comment, out *[]extraction.CommentInput) {
	if c.Depth > r.maxCommentDepth {
		return
	}
	if c.Score < r.minCommentScore {
		return
	}
	body := strings.TrimSpace(c.Body)
	if body == "" || body == "[deleted]" || body == "[removed]" {
		return
	}
	if c.Author == "AutoModerator" || c.Author == "[deleted]" {
		return
	}

	*out = append(*out, extraction.CommentInput{
		Author:      c.Author,
		Body:        body,
		Score:       c.Score,
		Depth:       c.Depth,
		IsSubmitter: c.IsSubmitter,
	})

	for _, reply := range c.Replies {
		r.flattenComment(reply, out)
	}
}
