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

	"github.com/teslashibe/verum-extract/aggregate"
	"github.com/teslashibe/verum-extract/anthropic"
	"github.com/teslashibe/verum-extract/compounds"
	"github.com/teslashibe/verum-extract/extraction"
	"github.com/teslashibe/verum-extract/normalize"
	"github.com/teslashibe/verum-extract/source/reddit"
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
		fmt.Printf("verum-extract v%s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`verum-extract v%s — extract structured peptide research data from community sources

Usage:
  verum-extract run       --input <file.jsonl> --output <dir>    Full pipeline
  verum-extract extract   --input <file.jsonl> --output <dir>    Extract only
  verum-extract aggregate --input <reports-dir> --output <dir>   Aggregate only
  verum-extract compounds [--category <cat>]                     List compounds
  verum-extract version                                          Show version
  verum-extract help                                             Show this help
`, version)
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	input := fs.String("input", "", "Input JSONL file")
	output := fs.String("output", "./output", "Output directory")
	apiKey := fs.String("anthropic-key", envOr("ANTHROPIC_API_KEY", ""), "Anthropic API key")
	model := fs.String("model", envOr("ANTHROPIC_MODEL", "claude-sonnet-4-6"), "Model name")
	batchSize := fs.Int("batch-size", 1000, "Requests per batch")
	salt := fs.String("author-salt", envOr("VERUM_AUTHOR_SALT", "verum-extract-v1"), "Author hash salt")
	compoundsFile := fs.String("compounds-file", envOr("VERUM_COMPOUNDS_FILE", ""), "Additional compounds JSON file")
	autoRegister := fs.Bool("auto-register", false, "Auto-register new compounds discovered by the LLM")
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
	fmt.Printf("verum-extract v%s\n\n", version)

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

	if *compoundsFile != "" {
		added, err := registry.LoadFile(*compoundsFile)
		if err != nil {
			fatal("load compounds file: %v", err)
		}
		fmt.Printf("      Loaded %d additional compounds from %s\n", added, *compoundsFile)
	}
	fmt.Printf("      Registry: %d compounds\n", registry.Len())

	var normOpts []normalize.NormalizerOption
	if *autoRegister {
		normOpts = append(normOpts, normalize.WithAutoRegister())
	}
	normalizer := normalize.New(registry, normOpts...)
	normStats := normalizer.NormalizeAll(extractResult.Reports)

	matchLine := fmt.Sprintf("      %d/%d matched (%.1f%%)", normStats.Matched, normStats.TotalCompounds, normStats.MatchRate*100)
	if normStats.AutoRegistered > 0 {
		matchLine += fmt.Sprintf(" | %d auto-registered", normStats.AutoRegistered)
	}
	if normStats.Unmatched > 0 {
		matchLine += fmt.Sprintf(" | %d unmatched", normStats.Unmatched)
	}
	fmt.Println(matchLine)
	fmt.Println()

	if len(normStats.UnmatchedNames) > 0 {
		data, _ := json.MarshalIndent(normStats.UnmatchedNames, "", "  ")
		os.WriteFile(filepath.Join(*output, "unmatched_compounds.json"), data, 0o644)
	}

	if normStats.AutoRegistered > 0 {
		registryPath := filepath.Join(*output, "compounds_registry.json")
		if err := registry.SaveFile(registryPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save expanded registry: %v\n", err)
		} else {
			fmt.Printf("      Saved expanded registry (%d compounds) → %s\n\n", registry.Len(), registryPath)
		}
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
	salt := fs.String("author-salt", envOr("VERUM_AUTHOR_SALT", "verum-extract-v1"), "Author hash salt")
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
	compoundsFile := fs.String("compounds-file", envOr("VERUM_COMPOUNDS_FILE", ""), "Additional compounds JSON file")
	autoReg := fs.Bool("auto-register", false, "Auto-register new compounds found in reports")
	fs.Parse(args)

	reports, err := aggregate.ReadReports(*input)
	if err != nil {
		fatal("read reports: %v", err)
	}
	fmt.Printf("Read %d reports\n", len(reports))

	registry := compounds.Default()
	if *compoundsFile != "" {
		added, err := registry.LoadFile(*compoundsFile)
		if err != nil {
			fatal("load compounds: %v", err)
		}
		fmt.Printf("Loaded %d additional compounds\n", added)
	}

	var normOpts []normalize.NormalizerOption
	if *autoReg {
		normOpts = append(normOpts, normalize.WithAutoRegister())
	}
	normalizer := normalize.New(registry, normOpts...)
	normStats := normalizer.NormalizeAll(reports)
	fmt.Printf("Normalized: %d/%d matched (%.1f%%)", normStats.Matched, normStats.TotalCompounds, normStats.MatchRate*100)
	if normStats.AutoRegistered > 0 {
		fmt.Printf(" | %d auto-registered", normStats.AutoRegistered)
	}
	fmt.Println()

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
	compoundsFile := fs.String("compounds-file", envOr("VERUM_COMPOUNDS_FILE", ""), "Additional compounds JSON file")
	fs.Parse(args)

	registry := compounds.Default()
	if *compoundsFile != "" {
		if added, err := registry.LoadFile(*compoundsFile); err != nil {
			fatal("load compounds: %v", err)
		} else {
			fmt.Printf("(loaded %d additional compounds from %s)\n\n", added, *compoundsFile)
		}
	}

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
