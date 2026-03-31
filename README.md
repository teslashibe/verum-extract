# peptidebase

Open-source LLM-powered pipeline for extracting structured peptide research data from community sources.

Takes raw Reddit posts (and soon YouTube, podcasts, blogs) and produces structured, queryable compound profiles backed by real-world community data.

## What it does

1. **Reads** Reddit JSONL files (from [reddit-scraper](https://github.com/teslashibe/reddit-scraper))
2. **Extracts** structured report records via Anthropic Batch API (50% cheaper than real-time)
3. **Normalizes** compound names against a built-in registry of 40+ peptides
4. **Aggregates** reports into per-compound profiles with stats
5. **Writes** clean JSON files — individual reports + compound profiles

## Quick Start

```bash
# Install
go install github.com/teslashibe/peptidebase/cmd/peptidebase@latest

# Set your Anthropic API key
export ANTHROPIC_API_KEY=sk-ant-...

# Run the full pipeline
peptidebase run --input r_Peptides_2026-03-30.jsonl --output ./output/
```

## CLI

```bash
# Full pipeline: ingest → extract → normalize → aggregate
peptidebase run --input <file.jsonl> --output <dir>

# Extract only (skip aggregation)
peptidebase extract --input <file.jsonl> --output <dir>

# Aggregate from existing report JSONs
peptidebase aggregate --input <reports-dir> --output <dir>

# Browse the built-in compound registry
peptidebase compounds
peptidebase compounds --category healing_recovery

# Version
peptidebase version
```

## Output

```
output/
├── reports/
│   ├── {uuid}.json              # Individual extracted report records
│   └── ...
├── compounds/
│   ├── bpc_157.json             # Aggregated compound profile
│   ├── semaglutide.json
│   └── ...
├── unmatched_compounds.json     # Compound names not in registry
└── run_summary.json             # Pipeline stats
```

## Use as a Library

Every package is importable:

```go
import (
    "github.com/teslashibe/peptidebase/anthropic"
    "github.com/teslashibe/peptidebase/compounds"
    "github.com/teslashibe/peptidebase/extraction"
    "github.com/teslashibe/peptidebase/normalize"
    "github.com/teslashibe/peptidebase/aggregate"
    "github.com/teslashibe/peptidebase/source/reddit"
)
```

## Architecture

```
source/reddit     →  extraction  →  normalize  →  aggregate  →  JSON files
(JSONL adapter)      (LLM core)     (registry)    (profiles)
```

- **`compounds/`** — Built-in registry of 40+ peptides with aliases and categories
- **`anthropic/`** — Anthropic Batch API client (reusable for any project)
- **`extraction/`** — Schema types, prompt templates, response parsing
- **`normalize/`** — Fuzzy compound name matching (Levenshtein + alias lookup)
- **`aggregate/`** — Profile computation from extracted reports
- **`source/reddit/`** — Reddit JSONL ingestion adapter

## Supported Sources

| Source | Status |
|--------|--------|
| Reddit | ✅ Working |
| YouTube | 🔜 Planned |
| Podcasts | 🔜 Planned |
| Blogs | 🔜 Planned |

## License

MIT
