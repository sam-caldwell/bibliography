package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"

	"bibliography/src/internal/schema"
)

// Generator abstracts YAML generation so tests can fake it.
type Generator interface {
	GenerateYAML(ctx context.Context, typ string, hints map[string]string) (schema.Entry, string, error)
}

// NewGeneratorFromEnv returns a real OpenAI-backed generator if OPENAI_API_KEY is set,
// otherwise returns an error. For tests, use NewFakeGenerator.
func NewGeneratorFromEnv(model string) (Generator, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("missing OPENAI_API_KEY")
	}
	return &openAIGenerator{model: model, apiKey: apiKey}, nil
}

// openAIGenerator implements the Generator using OpenAI Responses API.
type openAIGenerator struct {
	model  string
	apiKey string
}

func (g *openAIGenerator) GenerateYAML(ctx context.Context, typ string, hints map[string]string) (schema.Entry, string, error) {
	// NOTE: We implement a minimal client using environment and avoid bringing in
	// external SDKs for testability. If you prefer the SDK, swap this out.
	// Since network access may be restricted during tests, this path should not run in tests.

	sys := buildSystemPrompt()
	usr := buildUserPrompt(typ, hints)

	// Use curl to call OpenAI Responses API to keep deps minimal.
	// Endpoint: https://api.openai.com/v1/responses
	// We request a plain text output (YAML only) per the prompt contract.
	body := fmt.Sprintf(`{
      "model": %q,
      "input": [{"role":"system","content":%q},{"role":"user","content":%q}],
      "temperature": 0
    }`, g.model, sys, usr)

	cmd := []string{"curl", "-sS", "-X", "POST", "https://api.openai.com/v1/responses",
		"-H", "Content-Type: application/json",
		"-H", "Authorization: Bearer " + g.apiKey,
		"-d", body}

	// Execute command
	out, err := runCommand(ctx, cmd[0], cmd[1:])
	if err != nil {
		return schema.Entry{}, "", fmt.Errorf("openai request failed: %w", err)
	}
	// Parse response JSON to extract text. We keep a very small inline parser to avoid deps.
	// We search for "output_text" or aggregated text fields. To remain robust, also try to
	// find top-level "output[0].content[0].text" patterns if the API returns such shape.
	yamlText := extractText(out)
	if strings.TrimSpace(yamlText) == "" {
		return schema.Entry{}, "", fmt.Errorf("empty YAML from OpenAI")
	}
	var e schema.Entry
	if err := yaml.Unmarshal([]byte(yamlText), &e); err != nil {
		return schema.Entry{}, yamlText, fmt.Errorf("invalid YAML: %w", err)
	}
	if strings.TrimSpace(e.ID) == "" {
		// If model did not supply id, try to compute from title/year per policy
		e.ID = schema.Slugify(e.APA7.Title, e.APA7.Year)
	}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, yamlText, err
	}
	return e, yamlText, nil
}

// buildSystemPrompt returns the exact system prompt per spec.
func buildSystemPrompt() string {
	schemaYAML := `id: "<slug>"
type: "website|book|movie|article|report|dataset|software"
apa7:
  authors:
    - family: "Last"
      given: "F. M."
  year: 2025
  date: "YYYY-MM-DD"
  title: "Title Case"
  container_title: "Site/Publisher/Journal"
  edition: "2nd"
  publisher: "Publisher"
  publisher_location: "City, ST"
  journal: "Journal"
  volume: "12"
  issue: "3"
  pages: "45–60"
  doi: "10.xxxx/xxxx"
  isbn: "978-..."
  url: "https://..."
  accessed: "YYYY-MM-DD"
annotation:
  summary: "2–5 sentences neutral summary"
  keywords: ["k1","k2","k3"]`
	return "You are a precise bibliographic agent. Output ONLY valid YAML matching this schema:\n" + schemaYAML + "\nRules:\n- No markdown fences or extra prose.\n- Authors in APA (Family, Given initials).\n- ISO dates when known; otherwise omit optional fields.\n- id is a lowercase slug of title + year when present."
}

func buildUserPrompt(typ string, hints map[string]string) string {
	var b strings.Builder
	b.WriteString("Task: produce ONE YAML doc for type=")
	b.WriteString(typ)
	b.WriteString(" with the exact schema.\n")
	b.WriteString("Hints (may be partial; ensure factual correctness; fill missing fields if reliably derivable):\n")
	// Render hints as simple key: value lines
	for k, v := range hints {
		if strings.TrimSpace(v) == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}
	return b.String()
}

// runCommand is split for stubbing in tests if necessary.
var runCommand = func(ctx context.Context, name string, args []string) (string, error) {
	// Use os/exec directly here; we keep it inline to avoid extra test indirection for now.
	// Tests should avoid hitting this by using a FakeGenerator.
	// When called, we do a best-effort run.
	cmd := execCommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

// execCommandContext is wrapped for overdubbing in tests if needed.
var execCommandContext = func(ctx context.Context, name string, args ...string) command {
	return command{ctx: ctx, name: name, args: args}
}

// small wrapper to simplify mocking without importing os/exec directly in this file's API
type command struct {
	ctx  context.Context
	name string
	args []string
}

func (c command) Output() ([]byte, error) {
	// Lazy import to keep top tidy
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := execCommand(c.ctx, c.name, c.args...)
	return cmd.Output()
}

// indirection for os/exec.CommandContext
var execCommand = func(ctx context.Context, name string, args ...string) *osExecCmd { // wrapper type
	c := osExecCommand(name, args...)
	c.ctx = ctx
	return c
}

// A tiny wrapper around os/exec.Cmd to allow setting context lazily.
type osExecCmd struct {
	*realExecCmd
	ctx context.Context
}

// Below here, we provide bindings to os/exec. We keep them as vars for potential stubbing.
var (
	osExecCommand = func(name string, args ...string) *osExecCmd {
		rc := realCommand(name, args...)
		return &osExecCmd{realExecCmd: rc}
	}
	realCommand = func(name string, args ...string) *realExecCmd { return &realExecCmd{cmd: exec.Command(name, args...)} }
)

type realExecCmd struct{ cmd *exec.Cmd }

func (c *realExecCmd) Output() ([]byte, error) { return c.cmd.Output() }

// FakeGenerator implements Generator for tests.
type FakeGenerator struct {
	Entry schema.Entry
	Raw   string
	Err   error
}

func (f *FakeGenerator) GenerateYAML(ctx context.Context, typ string, hints map[string]string) (schema.Entry, string, error) {
	return f.Entry, f.Raw, f.Err
}

// extractText attempts to pull the model-generated YAML text out of the Responses API JSON.
// It tries several known shapes to be resilient to minor API shape differences.
func extractText(resp string) string {
	// Minimal parsing: look for fields commonly used: output_text, content[].text, etc.
	// Try to decode into a generic map first.
	var m map[string]any
	if err := json.Unmarshal([]byte(resp), &m); err == nil {
		// Check output_text as []string or string
		if v, ok := m["output_text"]; ok {
			switch t := v.(type) {
			case string:
				return t
			case []any:
				var b strings.Builder
				for _, it := range t {
					s, _ := it.(string)
					b.WriteString(s)
				}
				return b.String()
			}
		}
		// Check "output" array with nested content
		if out, ok := m["output"].([]any); ok {
			var b strings.Builder
			for _, item := range out {
				if mp, ok := item.(map[string]any); ok {
					if c, ok := mp["content"].([]any); ok {
						for _, ci := range c {
							if cm, ok := ci.(map[string]any); ok {
								if t, ok := cm["text"].(string); ok {
									b.WriteString(t)
								}
							}
						}
					}
				}
			}
			s := b.String()
			if strings.TrimSpace(s) != "" {
				return s
			}
		}
		// Fallback: check for top-level content array
		if c, ok := m["content"].([]any); ok {
			var b strings.Builder
			for _, ci := range c {
				if cm, ok := ci.(map[string]any); ok {
					if t, ok := cm["text"].(string); ok {
						b.WriteString(t)
					}
				}
			}
			s := b.String()
			if strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	// As last resort, return original string; caller will fail if not YAML.
	return resp
}
