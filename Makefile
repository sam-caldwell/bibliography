SHELL := /bin/bash

.PHONY: build fmt test cover cover-check clean tag tag/patch tag/minor tag/major

build: clean
	go fmt ./src/...
	go build -o bin/bib ./src/cmd/bib

fmt:
	go fmt ./src/...

test:
	go test ./src/... -coverprofile=coverage.out -covermode=atomic
	$(MAKE) cover-check

cover: test
	go tool cover -func=coverage.out | tail -n 1

# Verify total coverage is above threshold (default 80%).
# Usage: make cover-check [COVER_THRESHOLD=85]
cover-check:
	@set -euo pipefail; \
	if [ ! -f coverage.out ]; then \
	  echo "coverage.out not found. Run 'make test' first." >&2; \
	  exit 2; \
	fi; \
	pct=$$(go tool cover -func=coverage.out | awk '/^total:/ {gsub(/%/,"",$$NF); print $$NF}'); \
	thresh=$${COVER_THRESHOLD:-80}; \
	echo "Coverage: $$pct% (threshold $$thresh%)"; \
	awk -v p="$$pct" -v t="$$thresh" 'BEGIN { if ((p+0) < (t+0)) { exit 1 } }' || { \
	  echo "ERROR: coverage $$pct% is below threshold $$thresh%" >&2; \
	  exit 1; \
	}

clean:
	rm -rf bin
	rm -f coverage.out
	rm -f data/metadata/*.json

# Tag the current commit with the next semantic version and push the tag.
# Usage:
#   make tag                  # bumps patch (or creates v0.0.0 if none exist)
#   make tag LEVEL=minor      # bump minor (resets patch)
#   make tag LEVEL=major      # bump major (resets minor+patch)
#   make tag VERSION=v1.2.3   # force an explicit version tag
tag:
	@set -euo pipefail; \
	LATEST=$$(git tag --list 'v[0-9]*' --sort=-v:refname | head -n1 || true); \
	if [ -n "$${VERSION:-}" ]; then \
	  NEW="$$VERSION"; \
	else \
	  LVL="$${LEVEL:-}"; \
	  if [ -z "$$LATEST" ]; then \
	    if [ "$$LVL" = major ]; then NEW="v1.0.0"; \
	    elif [ "$$LVL" = minor ]; then NEW="v0.1.0"; \
	    elif [ "$$LVL" = patch ]; then NEW="v0.0.1"; \
	    else NEW="v0.0.0"; fi; \
	  else \
	    VER=$${LATEST#v}; \
	    MAJOR=$${VER%%.*}; REST=$${VER#*.}; \
	    MINOR=$${REST%%.*}; PATCH=$${REST#*.}; \
	    case "$${LVL:-patch}" in \
	      major) MAJOR=$$((MAJOR+1)); MINOR=0; PATCH=0 ;; \
	      minor) MINOR=$$((MINOR+1)); PATCH=0 ;; \
	      patch|*) PATCH=$$((PATCH+1)) ;; \
	    esac; \
	    NEW="v$${MAJOR}.$${MINOR}.$${PATCH}"; \
	  fi; \
	fi; \
	printf "Tagging %s\n" "$$NEW"; \
	git tag -a "$$NEW" -m "Release $$NEW"; \
	git push origin "$$NEW"

# Convenience targets: bump specific level
tag/patch:
	@$(MAKE) tag LEVEL=patch

tag/minor:
	@$(MAKE) tag LEVEL=minor

tag/major:
	@$(MAKE) tag LEVEL=major
