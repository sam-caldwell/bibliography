# Bibliography CLI (APA7 + Annotated YAML)

Version: [![Version](https://img.shields.io/github/v/tag/sam-caldwell/bibliography?sort=semver)](https://github.com/sam-caldwell/bibliography/tags)

(c) 2025 Sam Caldwell. (mail@samcaldwell.net). See [LICENSE](LICENSE)

![Bibliography CLI Logo](docs/logo.png)

Bibliography is a small, Git‑backed CLI for building an annotated bibliography. Each citation is a single YAML file
stored under `data/citations/` and validated against a compact APA7‑inspired schema. The tool fetches public
metadata when available (doi.org for articles; OpenLibrary→Google→Crossref→WorldCat→BNB→openBD→LoC→OpenAI for books by ISBN), supports interactive entry for everything
else, and keeps lightweight JSON indexes for discovery.

The CLI reads and writes only local files. It can also use OpenAI to summarize works and generate keywords, and it
falls back to OpenAI to build a citation when an article URL is blocked (HTTP 401/403).

--------------------------------------------------------------------------------
## The Contents
The cited works already added to this repo are listed in human-readable form [here](docs/index.html).

## Quick Start

Requirements

- Go 1.22+
- Git (with an `origin` remote and push credentials)
- Optional: `OPENAI_API_KEY` for summarization, keyword generation, and 401/403 article URL fallbacks

Build and list commands

```bash
make build
./bin/bib --help
```

Common commands

```bash
# Add a website (URL or fully manual prompts)
./bin/bib add site https://example.com
./bin/bib add site

# Add a book (ISBN lookup with multi-provider fallback; manual otherwise)
./bin/bib add book --isbn 9780132350884
./bin/bib add book --name "The Pragmatic Programmer" --author "Hunt, A."

# Add a movie (title/date or manual)
./bin/bib add movie "12 Angry Men" --date 1957-04-10

# Add an article by DOI (via doi.org)
./bin/bib add article --doi 10.1234/xyz

# Add an article by URL; on 401/403, fall back to OpenAI (requires OPENAI_API_KEY)
./bin/bib add article --url https://example.com/post

# Manual add for any type (prompts for required/optional fields)
./bin/bib add article

# Edit by id: print current YAML or update dot‑path values (values parsed as YAML)
./bin/bib edit --id <uuid>
./bin/bib edit --id <uuid> --apa7.title="New Title" --annotation.keywords='["alpha","beta"]'

# Search entries containing all keywords (AND, case‑insensitive)
./bin/bib search --keyword k1,k2

# Summarize missing/boilerplate annotation summaries and generate keywords via OpenAI
./bin/bib summarize

# Rebuild metadata and commit the changes
./bin/bib index

# Normalize article DOIs and doi.org URLs
./bin/bib repair-doi

# Migrate existing entries to UUIDv4 IDs (safe preview with --dry-run)
./bin/bib migrate-ids --dry-run
```

--------------------------------------------------------------------------------

Data Layout

- `data/citations/` — one YAML per work under a type directory
  - Articles: `data/citations/article/<uuid>.yaml`
  - Books:    `data/citations/books/<uuid>.yaml`
  - Movies:   `data/citations/movie/<uuid>.yaml`
  - Websites: `data/citations/site/<uuid>.yaml`
  - RFCs:     `data/citations/rfc/<uuid>.yaml`
- `data/metadata/` — generated indexes used for fast lookups
  - `keywords.json` — map: keyword → array of work paths
  - `authors.json`  — map: "Family, Given" or organization name → array of work paths
  - `titles.json`   — map: work path → array of tokenized title words
  - `isbn.json`     — map: work path → ISBN (books only)
  - `doi.json`      — map: work path → DOI (works with a DOI)

All metadata can be regenerated at any time using `bib index`.

--------------------------------------------------------------------------------

Citation Schema

```yaml
id: "2e722384-ccef-485e-994d-70de22254383"    # UUIDv4 (canonical 36‑char form)
type: "website|book|movie|article|report|dataset|software|rfc"
apa7:
  authors:                                     # flexible shapes supported (person or organization)
    - family: "Last"
      given: "F. M."
    - "National Automated Clearing House Association"   # corporate/organization author
    # also accepted:
    # - organization: "World Health Organization"
    # - name: "U.S. Department of Labor"
  year: 2025                                   # optional (derived from date if present)
  date: "YYYY-MM-DD"                           # optional
  title: "Title Case"
  container_title: "Site/Publisher/Journal"    # optional
  edition: "2nd"                               # optional
  publisher: "Publisher"                       # optional
  publisher_location: "City, ST"               # optional
  journal: "Journal"                            # optional
  volume: "12"                                  # optional
  issue: "3"                                    # optional
  pages: "45–60"                                # optional
  doi: "10.xxxx/xxxx"                           # optional
  isbn: "978-..."                               # optional
  url: "https://..."                            # optional
  accessed: "YYYY-MM-DD"                        # required if url is present
annotation:
  summary: "2–5 sentences of neutral prose"
  keywords: ["k1", "k2", "k3"]
```

Validation

- Required: `id` (UUIDv4), `type`, `apa7.title`, `annotation.summary`, `annotation.keywords` (non‑empty).
- If `apa7.url` is set, `apa7.accessed` must be present; the CLI sets it automatically during adds/edits.

--------------------------------------------------------------------------------

Add Flows

- `add book --isbn` attempts OpenLibrary first, then falls back in order to Google Books, Crossref REST, OCLC WorldCat (Classify), British National Bibliography (BNB) SPARQL, openBD (Japan), and the US Library of Congress.
- `add book --name <title> --author <family, given> --lookup` attempts an online lookup (OpenLibrary→Google Books→Crossref). Without `--lookup`, it constructs a basic entry from flags.
- `add article --doi` uses doi.org (CSL JSON). URL is set to `https://doi.org/<DOI>` and `accessed` is set.
- `add article --url` fetches the page with a Chrome‑like User‑Agent and extracts OpenGraph/JSON‑LD/PDF metadata.
  - If the server responds 401 or 403, the CLI falls back to OpenAI to generate a citation (requires
    `OPENAI_API_KEY`).
- Any `add` without sufficient flags runs an interactive prompt and validates inputs before writing YAML.

Editing

- `bib edit --id <uuid>` prints the current YAML.
- `bib edit --id <uuid> --field.path=value` updates a dot‑delimited path in the YAML.
  - Values are parsed as YAML (arrays/maps/scalars supported).
  - Changing `type` moves the file to the corresponding directory.
  - Setting `apa7.url` auto‑adds `apa7.accessed` if missing.

Indexing and Search

- `bib index` rebuilds all metadata files under `data/metadata/` and commits the result.
- Keyword index includes:
  - `annotation.keywords` and tokens from `annotation.summary` and `apa7.title`
  - Publisher and container/journal (full phrases and tokens)
  - Year, domain host (both `www.<host>` and `<host>`), and the work `type`
- `bib search --keyword k1,k2` returns works whose `annotation.keywords` contain both `k1` and `k2`.

Summaries and Keywords (OpenAI)

- `bib summarize` finds entries with missing or boilerplate summaries, checks that the URL is reachable, and asks
  OpenAI for a ~200‑word neutral summary. It then asks OpenAI for 5–12 concise topical keywords and merges them with
  existing keywords (sorted, de‑duplicated, lowercased).
- Set `OPENAI_API_KEY` to enable these calls. You can also set `OPENAI_MODEL` (defaults to `gpt-4o-mini`).

IDs and Migration

- New entries receive a UUIDv4 ID automatically.
- `bib migrate-ids` converts older entries to UUIDv4 and renames files accordingly. Use `--dry-run` to preview.

Git Behavior

- After a successful add/edit, the CLI stages the changed file, commits (`add citation: <id>` / `updated …`), and
  pushes.
- `bib index` stages the entire `data/metadata/` directory and commits (`index: rebuild metadata`), then pushes.
- If `bib index` is run outside a Git repo, it prints a warning and exits successfully without committing.

Configuration

- `OPENAI_API_KEY` — required for `summarize` and for the 401/403 fallback in `add article --url`.
- `OPENAI_MODEL` — optional model name, defaults to `gpt-4o-mini`.

Development

```bash
make test
make cover     # print total coverage
make fmt       # format the code
```

Troubleshooting

- Git errors during `index`:
  - Ensure you are inside a Git worktree with `origin` configured and valid credentials.
  - Some Git setups print “nothing to commit” to stdout with a non‑zero exit; the CLI treats that as success.
- Access‑denied web pages during `add article --url`:
  - The CLI automatically falls back to OpenAI if `OPENAI_API_KEY` is set; otherwise it returns the 401/403 error.
- Invalid YAML or validation errors:
  - The CLI fails fast and writes nothing. Fix inputs and re‑run.

Security & Data Ownership

- Citation YAMLs in `data/citations/` are the canonical source of truth checked into your repo.
- Everything in `data/metadata/` is derived and can be regenerated at any time.
- The CLI only uses public metadata endpoints for adds and never uploads repository contents.
