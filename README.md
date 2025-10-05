# Bibliography CLI (APA7 + Annotation)

A single-CLI, Git-backed annotated bibliography that stores one APA7 entry per YAML file. The tool looks up citations using public metadata providers (doi.org for articles; OpenLibrary for books), validates and writes them to `data/citations/`, keeps a keyword index in `data/metadata/`, and commits/pushes changes automatically. A GitHub Action keeps indexes consistent on every push.

## Highlights

- Simple CLI: `bib`
  - `lookup site|book|movie|article` generates a citation using doi.org/OpenLibrary or minimal user-provided metadata and writes YAML.
  - `search --keyword k1,k2` lists entries that contain all keywords (AND, case-insensitive).
  - `index` rebuilds the keyword index JSON.
- Deterministic output: strict YAML validated against a schema (no AI calls).
- Git integration: stages, commits, and pushes new/updated entries.
- CI workflow: rebuilds `data/metadata/keywords.json` on push and commits only if changed.

## Requirements

- Go 1.22+
- Git (with a configured `origin` remote and auth for pushes)
- `curl`
- Environment: none required for lookups

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
# Website by URL (minimal metadata)
./bin/bib lookup site https://example.com

# Book (via OpenLibrary when ISBN is provided)
./bin/bib lookup book --isbn 9780132350884
# Book (minimal metadata when no ISBN)
./bin/bib lookup book --name "The Pragmatic Programmer" --author "Hunt, A."

# Movie by title (and optional date)
./bin/bib lookup movie "12 Angry Men" --date 1957-04-10

# Article by DOI (via doi.org)
./bin/bib lookup article --doi 10.1234/xyz
# Article by metadata (minimal)
./bin/bib lookup article --title "Cool Result" --author "Doe, J." --journal "Science" --date 2023-04-12

# Optional keywords for any lookup
./bin/bib lookup book --isbn 9780132350884 --keywords "software,clean-code"
```

Search (AND semantics across keywords, case-insensitive):

```bash
./bin/bib search --keyword golang,apa7
```

Rebuild index:

```bash
./bin/bib index
```

## DOI behavior

When using `bib lookup article --doi <DOI>`:
- The CLI fetches metadata directly from doi.org (CSL JSON).
- It sets `apa7.url` to `https://doi.org/<DOI>` and `apa7.accessed` to today’s UTC date.

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
- If `apa7.url` is present, `apa7.accessed` must be present (the CLI sets `accessed` when needed).

## Git Agent

- After a successful lookup write, runs:
  1. `git add <path>`
  2. `git commit -m "add citation: <id>"`
  3. `git push` (sets upstream automatically if missing)
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

- Tests use local doubles for DOI and OpenLibrary HTTP calls and a local bare Git repo to cover behavior without external side effects.
- Current test coverage ≥ 80%.

## Troubleshooting

- Git push fails:
  - Ensure `origin` remote exists and credentials are available (SSH agent or PAT).
- Invalid YAML input:
  - The CLI fails fast and does not write partial files; correct inputs and re-run.

## Security & Data Ownership

- Citation YAMLs in `data/citations/` are the canonical source of truth.
- Metadata in `data/metadata/` is derived and can be regenerated at any time.
- The CLI only uses public metadata endpoints for lookups and never uploads repository contents.

## Project Structure

```
.
├── Makefile
├── .github/workflows/
│   ├── index.yml           # rebuild metadata
│   ├── test.yml            # run tests
│   └── coverage.yml        # coverage gate >= 80%
├── data/
│   ├── citations/          # one YAML per entry
│   └── metadata/           # generated metadata (keywords.json)
├── src/
│   ├── cmd/bib/            # Cobra root and subcommands
│   └── internal/
│       ├── doi/            # DOI resolver (CSL JSON via doi.org)
│       ├── openlibrary/    # OpenLibrary client for ISBN
│       ├── gitutil/        # git add/commit/push
│       ├── schema/         # entry structs + validation
│       └── store/          # reading/writing entries, indexing
└── specification-v.0.0.1.yaml
```

## Runbook (Quick)

1) Build

```bash
make build
```

2) Create a citation

```bash
./bin/bib lookup article --doi 10.1234/xyz
```

3) Verify a new file under `data/citations/` and a pushed commit on GitHub.

4) On the next push, CI will rebuild the metadata and push if anything changed.
