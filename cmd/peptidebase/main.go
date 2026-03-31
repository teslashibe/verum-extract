package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/teslashibe/peptidebase/aggregate"
	"github.com/teslashibe/peptidebase/anthropic"
	"github.com/teslashibe/peptidebase/compounds"
	"github.com/teslashibe/peptidebase/extraction"
	"github.com/teslashibe/peptidebase/normalize"
	"github.com/teslashibe/peptidebase/source/reddit"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "extract":
		cmdExtract(os.Args[2:])
	case "aggregate":
		cmdAggregate(os.Args[2:])
	case "compounds":
		cmdCompounds(os.Args[2:])
	case "version":
		fmt.Printf("peptidebase v%s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`peptidebase v%s — extract structured peptide research data from community sources

Usage:
  peptidebase run       --input <file.jsonl> --output <dir>    Full pipeline
  peptidebase extract   --input <file.jsonl> --output <dir>    Extract only
  peptidebase aggregate --input <reports-dir> --output <dir>   Aggregate only
  peptidebase compounds [--category <cat>]                     List compounds
  peptidebase version                                          Show version
  peptidebase help                                             Show this help
`, version)
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	input := fs.String("input", "", "Input JSONL file")
	output := fs.String("output", "./output", "Output directory")
	apiKey := fs.String("anthropic-key", envOr("ANTHROPIC_API_KEY", ""), "Anthropic API key")
	model := fs.String("model", envOr("ANTHROPIC_MODEL", "claude-sonnet-4-6"), "Model name")
	batchSize := fs.Int("batch-size", 1000, "Requests per batch")
	salt := fs.String("author-salt", envOr("PEPTIDEBASE_AUTHOR_SALT", "peptidebase-v1"), "Author hash salt")
	verbose := fs.Bool("verbose", false, "Verbose logging")
	fs.Parse(args)

	if *input == "" {
		fatal("--input is required")
	}
	if *apiKey == "" {
		fatal("--anthropic-key or ANTHROPIC_API_KEY is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	reportsDir := filepath.Join(*output, "reports")
	compoundsDir := filepath.Join(*output, "compounds")
	os.MkdirAll(reportsDir, 0o755)
	os.MkdirAll(compoundsDir, 0o755)

	start := time.Now()
	fmt.Printf("peptidebase v%s\n\n", version)

	// Step 1: Ingest
	fmt.Println("[1/4] Reading", *input, "...")
	reader := reddit.NewReader()
	inputs, readStats, err := reader.ReadFile(*input)
	if err != nil {
		fatal("read file: %v", err)
	}
	fmt.Printf("      %d lines → %d posts (%d skipped", readStats.TotalLines, readStats.Parsed, readStats.Skipped)
	if len(readStats.SkipReasons) > 0 {
		var reasons []string
		for k, v := range readStats.SkipReasons {
			reasons = append(reasons, fmt.Sprintf("%d %s", v, k))
		}
		fmt.Printf(": %s", strings.Join(reasons, ", "))
	}
	fmt.Printf(")\n\n")

	// Step 2: Extract
	fmt.Println("[2/4] Extracting reports via Anthropic Batch API...")
	fmt.Printf("      Model: %s | Batch size: %d\n", *model, *batchSize)

	client := anthropic.New(*apiKey, anthropic.WithModel(*model))
	extractor := extraction.NewExtractor(client,
		extraction.WithAuthorSalt(*salt),
		extraction.WithBatchSize(*batchSize),
	)

	extractResult, err := extractor.ExtractBatch(ctx, inputs, func(msg string) {
		fmt.Printf("      %s\n", msg)
	})
	if err != nil {
		fatal("extraction: %v", err)
	}

	for _, r := range extractResult.Reports {
		data, _ := json.MarshalIndent(r, "", "  ")
		os.WriteFile(filepath.Join(reportsDir, r.ID+".json"), data, 0o644)
	}

	avgConf := avgConfidence(extractResult.Reports)
	fmt.Printf("      Extracted %d reports from %d posts (%d failed)\n", len(extractResult.Reports), extractResult.Succeeded, extractResult.Failed)
	fmt.Printf("      Avg confidence: %.2f\n\n", avgConf)

	if *verbose && len(extractResult.Errors) > 0 {
		for _, e := range extractResult.Errors {
			fmt.Printf("      ERROR [%s]: %s\n", e.SourceID, e.Error)
		}
		fmt.Println()
	}

	// Step 3: Normalize
	fmt.Println("[3/4] Normalizing compound names...")
	registry := compounds.Default()
	normalizer := normalize.New(registry)
	normStats := normalizer.NormalizeAll(extractResult.Reports)
	fmt.Printf("      %d/%d matched (%.1f%%) | %d unmatched\n\n",
		normStats.Matched, normStats.TotalCompounds, normStats.MatchRate*100, normStats.Unmatched)

	if len(normStats.UnmatchedNames) > 0 {
		data, _ := json.MarshalIndent(normStats.UnmatchedNames, "", "  ")
		os.WriteFile(filepath.Join(*output, "unmatched_compounds.json"), data, 0o644)
	}

	for _, r := range extractResult.Reports {
		data, _ := json.MarshalIndent(r, "", "  ")
		os.WriteFile(filepath.Join(reportsDir, r.ID+".json"), data, 0o644)
	}

	// Step 4: Aggregate
	fmt.Println("[4/4] Aggregating compound profiles...")
	aggResult := aggregate.FromReports(extractResult.Reports, registry)
	if err := aggregate.WriteProfiles(aggResult.Profiles, compoundsDir); err != nil {
		fatal("write profiles: %v", err)
	}

	limited := 0
	for _, p := range aggResult.Profiles {
		if p.LimitedData {
			limited++
		}
	}
	fmt.Printf("      %d compounds profiled (%d limited data)\n\n", aggResult.CompoundsProfiled, limited)

	// Summary
	elapsed := time.Since(start)
	summary := map[string]any{
		"pipeline_version": version,
		"started_at":       start.UTC(),
		"completed_at":     time.Now().UTC(),
		"duration_seconds":  int(elapsed.Seconds()),
		"input_file":       *input,
		"source_type":      "reddit",
		"model":            *model,
		"ingestion":        readStats,
		"extraction": map[string]any{
			"batch_ids":         extractResult.BatchIDs,
			"succeeded":         extractResult.Succeeded,
			"failed":            extractResult.Failed,
			"reports_extracted":  len(extractResult.Reports),
			"avg_confidence":    avgConf,
		},
		"normalization": normStats,
		"aggregation": map[string]any{
			"compounds_found":        aggResult.CompoundsFound,
			"compounds_profiled":     aggResult.CompoundsProfiled,
			"compounds_limited_data": limited,
		},
	}
	summaryJSON, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(filepath.Join(*output, "run_summary.json"), summaryJSON, 0o644)

	fmt.Printf("✓ Complete in %s\n", elapsed.Round(time.Second))
	fmt.Printf("  Reports:   %s (%d files)\n", reportsDir, len(extractResult.Reports))
	fmt.Printf("  Compounds: %s (%d files)\n", compoundsDir, aggResult.CompoundsProfiled)
	fmt.Printf("  Summary:   %s\n", filepath.Join(*output, "run_summary.json"))
}

func cmdExtract(args []string) {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	input := fs.String("input", "", "Input JSONL file")
	output := fs.String("output", "./output/reports", "Output directory for report JSONs")
	apiKey := fs.String("anthropic-key", envOr("ANTHROPIC_API_KEY", ""), "Anthropic API key")
	model := fs.String("model", envOr("ANTHROPIC_MODEL", "claude-sonnet-4-6"), "Model name")
	batchSize := fs.Int("batch-size", 1000, "Requests per batch")
	salt := fs.String("author-salt", envOr("PEPTIDEBASE_AUTHOR_SALT", "peptidebase-v1"), "Author hash salt")
	fs.Parse(args)

	if *input == "" || *apiKey == "" {
		fatal("--input and --anthropic-key are required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.MkdirAll(*output, 0o755)

	reader := reddit.NewReader()
	inputs, stats, err := reader.ReadFile(*input)
	if err != nil {
		fatal("read: %v", err)
	}
	fmt.Printf("Read %d posts (%d skipped)\n", stats.Parsed, stats.Skipped)

	client := anthropic.New(*apiKey, anthropic.WithModel(*model))
	extractor := extraction.NewExtractor(client,
		extraction.WithAuthorSalt(*salt),
		extraction.WithBatchSize(*batchSize),
	)

	result, err := extractor.ExtractBatch(ctx, inputs, func(msg string) {
		fmt.Println(msg)
	})
	if err != nil {
		fatal("extraction: %v", err)
	}

	for _, r := range result.Reports {
		data, _ := json.MarshalIndent(r, "", "  ")
		os.WriteFile(filepath.Join(*output, r.ID+".json"), data, 0o644)
	}
	fmt.Printf("Extracted %d reports → %s\n", len(result.Reports), *output)
}

func cmdAggregate(args []string) {
	fs := flag.NewFlagSet("aggregate", flag.ExitOnError)
	input := fs.String("input", "./output/reports", "Input reports directory")
	output := fs.String("output", "./output/compounds", "Output directory")
	fs.Parse(args)

	reports, err := aggregate.ReadReports(*input)
	if err != nil {
		fatal("read reports: %v", err)
	}
	fmt.Printf("Read %d reports\n", len(reports))

	registry := compounds.Default()
	normalizer := normalize.New(registry)
	normStats := normalizer.NormalizeAll(reports)
	fmt.Printf("Normalized: %d/%d matched (%.1f%%)\n", normStats.Matched, normStats.TotalCompounds, normStats.MatchRate*100)

	result := aggregate.FromReports(reports, registry)
	os.MkdirAll(*output, 0o755)
	if err := aggregate.WriteProfiles(result.Profiles, *output); err != nil {
		fatal("write: %v", err)
	}
	fmt.Printf("Wrote %d compound profiles → %s\n", result.CompoundsProfiled, *output)
}

func cmdCompounds(args []string) {
	fs := flag.NewFlagSet("compounds", flag.ExitOnError)
	category := fs.String("category", "", "Filter by category")
	fs.Parse(args)

	registry := compounds.Default()
	var list []compounds.Compound
	if *category != "" {
		list = registry.ByCategory(compounds.Category(*category))
	} else {
		list = registry.All()
	}

	fmt.Printf("%-20s %-20s %-20s %s\n", "ID", "NAME", "CATEGORY", "ALIASES")
	fmt.Println(strings.Repeat("-", 90))
	for _, c := range list {
		aliases := strings.Join(c.Aliases, ", ")
		if len(aliases) > 40 {
			aliases = aliases[:37] + "..."
		}
		fmt.Printf("%-20s %-20s %-20s %s\n", c.ID, c.DisplayName, c.Category, aliases)
	}
	fmt.Printf("\nTotal: %d compounds\n", len(list))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func avgConfidence(reports []extraction.Report) float64 {
	if len(reports) == 0 {
		return 0
	}
	var sum float64
	for _, r := range reports {
		sum += r.LLMConfidence
	}
	return sum / float64(len(reports))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
