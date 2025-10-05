# Bibliography CLI (APA7 + Annotation)

A single-CLI, Git-backed annotated bibliography that stores one APA7 entry per YAML file. The tool can look up 
citations via OpenAI, validate and write them to `data/citations/`, keep a keyword index in `data/metadata/`, and 
commit/push changes automatically. A GitHub Action keeps indexes consistent on every push.

## Highlights

- Simple CLI: `bib`
  - `lookup site|book|movie|article` generates a citation via OpenAI and writes YAML.
  - `search --keyword k1,k2` lists entries that contain all keywords (AND, case-insensitive).
  - `index` rebuilds the keyword index JSON.
- Deterministic AI agent: strict YAML-only output validated against a schema.
- Git integration: stages, commits, and pushes new/updated entries.
- CI workflow: rebuilds `data/metadata/keywords.json` on push and commits only if changed.

## Requirements

- Go 1.25+
- Git (with a configured `origin` remote and auth for pushes)
- `curl` (used by the OpenAI call)
- Environment:
  - `OPENAI_API_KEY` (required)
  - `BIBLIOGRAPHY_MODEL` (optional; default `gpt-4.1-mini`)

## Build

```bash
make build
./bin/bib --help
```

Optional targets:
- `make test` — run tests
- `make cover` — run tests and show total coverage
- `make fmt` — format the code
- `make clean` — remove build artifacts and generated metadata

## Data Layout

- `data/citations/` — one YAML file per entry: `data/citations/<id>.yaml`
- `data/metadata/` — generated metadata (e.g., `keywords.json`)

## CLI Usage

Lookup (creates a YAML, commits, and pushes):

```bash
# Website by URL
./bin/bib lookup site https://example.com

# Book by flags
./bin/bib lookup book --isbn 9780132350884
./bin/bib lookup book --name "The Pragmatic Programmer" --author "Hunt, A."

# Movie by title (and optional date)
./bin/bib lookup movie "12 Angry Men" --date 1957-04-10

# Article by DOI or metadata
./bin/bib lookup article --doi 10.1234/xyz
./bin/bib lookup article --title "Cool Result" --author "Doe, J." --journal "Science" --date 2023-04-12
```

Search (AND semantics across keywords, case-insensitive):

```bash
./bin/bib search --keyword golang,apa7
```

Rebuild index:

```bash
./bin/bib index
```

## OpenAI Bibliographic Agent

- Uses the Responses API to produce exactly one YAML document (no markdown fences).
- Prompt enforces APA7-style author formatting and an exact schema.
- The CLI strictly unmarshals and validates the YAML before writing.
- `BIBLIOGRAPHY_MODEL` can override the default model.
- Only user-provided hints (URL/DOI/title/author/etc.) are sent — the repo contents are not exfiltrated.

### DOI behavior

When using `bib lookup article --doi <DOI>`:
- If the model does not include a URL, the CLI sets `apa7.url` to `https://doi.org/<DOI>`.
- If a URL is present but `apa7.accessed` is missing, the CLI auto-fills `apa7.accessed` with today’s UTC date.

## Entry Schema (YAML)

```yaml
id: "<slug>"
type: "website|book|movie|article|report|dataset|software"
apa7:
  authors:
    - family: "Last"
      given: "F. M."
  year: 2025         # optional
  date: "YYYY-MM-DD" # optional
  title: "Title Case"
  container_title: "Site/Publisher/Journal" # optional
  edition: "2nd"                           # optional
  publisher: "Publisher"                   # optional
  publisher_location: "City, ST"           # optional
  journal: "Journal"                        # optional
  volume: "12"                              # optional
  issue: "3"                                # optional
  pages: "45–60"                            # optional
  doi: "10.xxxx/xxxx"                       # optional
  isbn: "978-..."                           # optional
  url: "https://..."                        # optional
  accessed: "YYYY-MM-DD"                    # required if url
annotation:
  summary: "2–5 sentences neutral summary"
  keywords: ["k1","k2","k3"]
```

### ID Policy
- `id` is a slug of the lowercase title with non-alphanumerics → `-`.
- Append `-YYYY` if the year is known.
- Collapse duplicate dashes and trim leading/trailing dashes.

### Validation Rules
- Required: `id`, `type`, `apa7.title`, `annotation.summary`, `annotation.keywords` (non-empty).
- If `apa7.url` is present, `apa7.accessed` must be present (the CLI fills `accessed` when generating entries).

## Git Agent

- After a successful lookup write, runs:
  1. `git add <path>`
  2. `git commit -m "add citation: <id>"`
  3. `git push`
- Treats “nothing to commit”/“no changes added to commit” as success.
- Requires a configured `origin` and valid push credentials (SSH agent or PAT).

## CI (GitHub Actions)

- Workflow: `.github/workflows/index.yml`
- Steps: checkout → setup Go → build CLI → `./bin/bib index` → if changed, commit `data/metadata/` and push.
- Idempotent: only commits when working tree is dirty.
- Permissions: `contents: write` for pushing changes.

## Testing

```bash
make test
make cover   # prints total coverage
```

- Tests use a fake AI generator and local bare Git repos to cover behavior without network and remote side effects.
- Current test coverage ≥ 80%.

## Troubleshooting

- Missing API key:
  - Error: `OPENAI_API_KEY not set; please export it to use lookup`
  - Fix: `export OPENAI_API_KEY=...`
- Git push fails:
  - Ensure `origin` remote exists and credentials are available (SSH agent or PAT).
- Invalid YAML from AI:
  - The CLI fails fast and does not write partial files; try refining hints or re-running.

## Security & Data Ownership

- Citation YAMLs in `data/citations/` are the canonical source of truth.
- Metadata in `data/metadata/` is derived and can be regenerated at any time.
- The CLI only sends explicit hints to OpenAI and never uploads repository contents.

## Project Structure

```
.
├── Makefile
├── .github/workflows/index.yml
├── data/
│   ├── citations/           # one YAML per entry
│   └── metadata/            # generated metadata (keywords.json)
├── src/
│   ├── cmd/bib/             # Cobra root and subcommands
│   └── internal/
│       ├── ai/              # OpenAI generator
│       ├── gitutil/         # git add/commit/push
│       ├── schema/          # entry structs + validation
│       └── store/           # reading/writing entries, indexing
└── specification-v.0.0.1.yaml
```

## Runbook (Quick)

1) Export `OPENAI_API_KEY` (and optional `BIBLIOGRAPHY_MODEL`).

```bash
export OPENAI_API_KEY=sk-...
export BIBLIOGRAPHY_MODEL=gpt-4.1-mini   # optional
```

2) Build

```bash
make build
```

3) Create a citation

```bash
./bin/bib lookup site https://example.com
```

4) Verify a new file under `data/citations/` and a pushed commit on GitHub.

5) On the next push, CI will rebuild the metadata and push if anything changed.

