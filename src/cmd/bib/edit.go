package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
)

// newEditCmd provides: bib edit --id <uuid> [--field.path=value ...]
//   - Without field assignments, prints the YAML to stdout.
//   - With one or more --field.path=value flags, applies updates using dot-delimited paths,
//     validates, and writes back (moving file if type changed).
// newEditCmd creates the "edit" command to display or update a citation by id.
func newEditCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:                "edit",
        Short:              "Show or update a citation YAML by id",
        DisableFlagParsing: true, // we manually parse to allow arbitrary --field=value flags
        RunE: func(cmd *cobra.Command, args []string) error { return executeEdit(cmd, args) },
    }
    return cmd
}

func executeEdit(cmd *cobra.Command, args []string) error {
    id, assignments, err := parseEditArgs(args)
    if err != nil { return err }
    if err := requireID(id); err != nil { return err }
    oldPath, err := locatePathForID(id)
    if err != nil { return err }

    if len(assignments) == 0 {
        return printFile(cmd, oldPath)
    }

    if err := disallowIDEdits(assignments); err != nil { return err }
    root, err := loadRootMapping(oldPath)
    if err != nil { return err }
    if err := applyAssignmentsToRoot(root, assignments); err != nil { return err }

    e, err := decodeEntry(root)
    if err != nil { return err }
    ensureURLAccessed(&e)
    if err := e.Validate(); err != nil { return err }
    return writeAndReport(cmd, oldPath, e)
}

func requireID(id string) error {
    if strings.TrimSpace(id) == "" {
        return fmt.Errorf("--id <uuid> is required")
    }
    return nil
}

func locatePathForID(id string) (string, error) {
    path, err := findPathByID(id)
    if err != nil { return "", err }
    if strings.TrimSpace(path) == "" {
        return "", fmt.Errorf("no citation found for id %s", id)
    }
    return path, nil
}

func printFile(cmd *cobra.Command, path string) error {
    b, err := os.ReadFile(path)
    if err != nil { return err }
    if _, err := fmt.Fprint(cmd.OutOrStdout(), string(b)); err != nil { return err }
    return nil
}

func disallowIDEdits(assignments map[string]string) error {
    for k := range assignments {
        if k == "id" || strings.HasPrefix(k, "id.") {
            return fmt.Errorf("editing 'id' is not supported")
        }
    }
    return nil
}

func loadRootMapping(path string) (*yaml.Node, error) {
    var doc yaml.Node
    b, err := os.ReadFile(path)
    if err != nil { return nil, err }
    if err := yaml.Unmarshal(b, &doc); err != nil {
        return nil, fmt.Errorf("invalid YAML in %s: %w", path, err)
    }
    if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
        return nil, fmt.Errorf("unexpected YAML structure in %s", path)
    }
    root := doc.Content[0]
    if root.Kind != yaml.MappingNode {
        return nil, fmt.Errorf("expected mapping at document root in %s", path)
    }
    return root, nil
}

func applyAssignmentsToRoot(root *yaml.Node, assignments map[string]string) error {
    for path, val := range assignments {
        if err := setYAMLPathValue(root, path, val); err != nil {
            return fmt.Errorf("set %s: %w", path, err)
        }
    }
    return nil
}

func decodeEntry(root *yaml.Node) (schema.Entry, error) {
    var e schema.Entry
    if err := root.Decode(&e); err != nil {
        return schema.Entry{}, fmt.Errorf("decode updated YAML: %w", err)
    }
    return e, nil
}

func ensureURLAccessed(e *schema.Entry) {
    if strings.TrimSpace(e.APA7.URL) != "" && strings.TrimSpace(e.APA7.Accessed) == "" {
        e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
    }
}

func writeAndReport(cmd *cobra.Command, oldPath string, e schema.Entry) error {
    newPath, err := store.WriteEntry(e)
    if err != nil { return err }
    oldPathSlash := filepath.ToSlash(oldPath)
    newPathSlash := filepath.ToSlash(newPath)
    if oldPathSlash != newPathSlash {
        _ = os.Remove(oldPath)
        if _, err := fmt.Fprintf(cmd.OutOrStdout(), "moved %s -> %s\n", oldPathSlash, newPathSlash); err != nil { return err }
    }
    if _, err := fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", newPathSlash); err != nil { return err }
    return nil
}

// parseEditArgs extracts --id and any --field.path=value assignments from args.
// parseEditArgs extracts an id and arbitrary --path=value assignments from args.
func parseEditArgs(args []string) (id string, assigns map[string]string, err error) {
    assigns = map[string]string{}
    for i := 0; i < len(args); i++ {
        a := args[i]

        // Handle id forms first: --id <val> or --id=val
        if a == "--id" {
            if i+1 >= len(args) {
                return "", nil, fmt.Errorf("--id requires a value")
            }
            id = args[i+1]
            i++
            continue
        }
        if strings.HasPrefix(a, "--id=") {
            id = strings.TrimPrefix(a, "--id=")
            continue
        }

        // Flag-style assignments: --key=value or --key value
        if strings.HasPrefix(a, "--") {
            if ni, ok := parseFlagAssignment(args, i, assigns); ok {
                i = ni
            }
            // Regardless of parse result, move to next token
            continue
        }

        // Bare key=value assignment
        _ = parseBareAssignment(a, assigns)
    }
    return id, assigns, nil
}

// parseFlagAssignment parses --key=value or --key value at args[i].
// Returns the new index and true if an assignment was captured (and possibly consumed the next arg).
func parseFlagAssignment(args []string, i int, assigns map[string]string) (int, bool) {
    a := args[i]
    // --key=value
    if eq := strings.IndexByte(a, '='); eq > 2 {
        key := strings.TrimSpace(a[2:eq])
        val := a[eq+1:]
        if key != "" {
            assigns[key] = val
            return i, true
        }
        return i, false
    }
    // --key value
    key := strings.TrimPrefix(a, "--")
    if key != "" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
        assigns[key] = args[i+1]
        return i + 1, true
    }
    return i, false
}

// parseBareAssignment parses key=value forms in a single token.
func parseBareAssignment(a string, assigns map[string]string) bool {
    if eq := strings.IndexByte(a, '='); eq > 0 {
        key := strings.TrimSpace(a[:eq])
        val := a[eq+1:]
        if key != "" {
            assigns[key] = val
            return true
        }
    }
    return false
}

// findPathByID returns the first matching YAML filepath for the given id.
// findPathByID returns the YAML filepath for the given id, scanning segmentation layouts.
func findPathByID(id string) (found string, err error) {
	// Support both segmented (type subdir) and flat layout
	// 1) segmented
	patterns := []string{
		filepath.Join(store.CitationsDir, "*", id+".yaml"),
		filepath.Join(store.CitationsDir, id+".yaml"),
	}
	for _, pat := range patterns {
		if matches, _ := filepath.Glob(pat); len(matches) > 0 {
			return matches[0], nil
		}
	}
	// Fallback: walk and match basename
	err = filepath.WalkDir(store.CitationsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(d.Name(), id+".yaml") {
			found = path
			return fmt.Errorf("stop")
		}
		return nil
	})
	return found, err
}

// setYAMLPathValue sets or creates the value at a dot-delimited path within the given mapping node.
// The value string is parsed as YAML, allowing scalars, sequences, or mappings.
// setYAMLPathValue updates/creates the node at dotPath within root using raw YAML value.
func setYAMLPathValue(root *yaml.Node, dotPath string, raw string) error {
    if root == nil || root.Kind != yaml.MappingNode {
        return fmt.Errorf("root must be a mapping node")
    }
    parts, err := splitDotPath(dotPath)
    if err != nil {
        return err
    }
    // Walk/create intermediate maps
    cur := root
    for i := 0; i < len(parts)-1; i++ {
        cur = getOrCreateChildMap(cur, parts[i])
    }
    // Set final key to parsed node value
    val := parseRawNode(raw)
    setMapKV(cur, parts[len(parts)-1], val)
    return nil
}

// splitDotPath splits a.b.c and validates non-empty segments.
// splitDotPath splits a dot-delimited path and errors on empty segments.
func splitDotPath(p string) ([]string, error) {
    segs := strings.Split(p, ".")
    out := make([]string, 0, len(segs))
    for _, s := range segs {
        s = strings.TrimSpace(s)
        if s == "" {
            return nil, fmt.Errorf("empty path segment in %q", p)
        }
        out = append(out, s)
    }
    return out, nil
}

// valueIndex returns the index of the value node for key in a mapping node,
// or -1 if not present.
// valueIndex returns the value index for key within a mapping node or -1 if missing.
func valueIndex(m *yaml.Node, key string) int {
    for i := 0; i+1 < len(m.Content); i += 2 {
        k := m.Content[i]
        if k.Kind == yaml.ScalarNode && k.Value == key {
            return i + 1
        }
    }
    return -1
}

// ensureMap coerces n to a mapping node in-place and returns it.
// ensureMap coerces the provided node into a mapping node and returns it.
func ensureMap(n *yaml.Node) *yaml.Node {
    if n.Kind != yaml.MappingNode {
        n.Kind = yaml.MappingNode
        n.Tag = "!!map"
        n.Content = nil
    }
    return n
}

// getOrCreateChildMap returns the mapping value for key in parent, creating it if missing.
// getOrCreateChildMap returns the mapping value for key, creating a mapping node when absent.
func getOrCreateChildMap(parent *yaml.Node, key string) *yaml.Node {
    if parent == nil {
        return nil
    }
    vi := valueIndex(parent, key)
    if vi >= 0 {
        return ensureMap(parent.Content[vi])
    }
    // add new mapping child
    k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
    v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
    parent.Content = append(parent.Content, k, v)
    return v
}

// setMapKV sets or appends a key/value in mapping node.
// setMapKV sets or appends a key/value pair in the mapping node.
func setMapKV(m *yaml.Node, key string, val *yaml.Node) {
    if m == nil { return }
    if idx := valueIndex(m, key); idx >= 0 {
        m.Content[idx] = val
        return
    }
    k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
    m.Content = append(m.Content, k, val)
}

// parseRawNode returns a node parsed from YAML input, or a string scalar on failure.
// parseRawNode parses raw YAML into a node or returns a scalar node of the raw string on failure.
func parseRawNode(raw string) *yaml.Node {
    var doc yaml.Node
    if err := yaml.Unmarshal([]byte(raw), &doc); err == nil && len(doc.Content) > 0 {
        return doc.Content[0]
    }
    return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: raw}
}
