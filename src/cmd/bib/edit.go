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
func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "edit",
		Short:              "Show or update a citation YAML by id",
		DisableFlagParsing: true, // we manually parse to allow arbitrary --field=value flags
		RunE: func(cmd *cobra.Command, args []string) error {
			id, assignments, err := parseEditArgs(args)
			if err != nil {
				return err
			}
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("--id <uuid> is required")
			}
			// Locate the YAML file path for this id.
			oldPath, err := findPathByID(id)
			if err != nil {
				return err
			}
			if oldPath == "" {
				return fmt.Errorf("no citation found for id %s", id)
			}

			// If no assignments, just print the YAML contents
			if len(assignments) == 0 {
				b, err := os.ReadFile(oldPath)
				if err != nil {
					return err
				}
				if _, err := fmt.Fprint(cmd.OutOrStdout(), string(b)); err != nil {
					return err
				}
				return nil
			}

			// Disallow editing the id directly
			for k := range assignments {
				if k == "id" || strings.HasPrefix(k, "id.") {
					return fmt.Errorf("editing 'id' is not supported; use migrate-ids for renumbering")
				}
			}

			// Load YAML as an AST
			var doc yaml.Node
			b, err := os.ReadFile(oldPath)
			if err != nil {
				return err
			}
			if err := yaml.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("invalid YAML in %s: %w", oldPath, err)
			}
			if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
				return fmt.Errorf("unexpected YAML structure in %s", oldPath)
			}
			root := doc.Content[0]
			if root.Kind != yaml.MappingNode {
				return fmt.Errorf("expected mapping at document root in %s", oldPath)
			}

			// Apply assignments
			for path, val := range assignments {
				if err := setYAMLPathValue(root, path, val); err != nil {
					return fmt.Errorf("set %s: %w", path, err)
				}
			}

			// Decode into schema.Entry for validation and canonical writing
			var e schema.Entry
			if err := root.Decode(&e); err != nil {
				return fmt.Errorf("decode updated YAML: %w", err)
			}
			// Ensure accessed date if URL present
			if strings.TrimSpace(e.APA7.URL) != "" && strings.TrimSpace(e.APA7.Accessed) == "" {
				e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
			}
			// Validate entry
			if err := e.Validate(); err != nil {
				return err
			}
			// Write via store (may move file if type changed)
			newPath, err := store.WriteEntry(e)
			if err != nil {
				return err
			}
			// If path changed (e.g., type changed), remove old file
			oldPathSlash := filepath.ToSlash(oldPath)
			newPathSlash := filepath.ToSlash(newPath)
			if oldPathSlash != newPathSlash {
				_ = os.Remove(oldPath)
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "moved %s -> %s\n", oldPathSlash, newPathSlash)
				if err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", newPathSlash)
			if err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

// parseEditArgs extracts --id and any --field.path=value assignments from args.
func parseEditArgs(args []string) (id string, assigns map[string]string, err error) {
	assigns = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
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
		if strings.HasPrefix(a, "--") {
			// unknown flag or assignment
			if eq := strings.IndexByte(a, '='); eq > 2 {
				key := strings.TrimSpace(a[2:eq])
				val := a[eq+1:]
				if key != "" {
					assigns[key] = val
				}
				continue
			}
			// Support "--field value" form
			key := strings.TrimPrefix(a, "--")
			if key != "" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				assigns[key] = args[i+1]
				i++
				continue
			}
			continue
		}
		// Support bare key=value
		if eq := strings.IndexByte(a, '='); eq > 0 {
			key := strings.TrimSpace(a[:eq])
			val := a[eq+1:]
			if key != "" {
				assigns[key] = val
			}
		}
	}
	return id, assigns, nil
}

// findPathByID returns the first matching YAML filepath for the given id.
func findPathByID(id string) (string, error) {
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
	var found string
	_ = filepath.WalkDir(store.CitationsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(d.Name(), id+".yaml") {
			found = path
			return fmt.Errorf("stop")
		}
		return nil
	})
	return found, nil
}

// setYAMLPathValue sets or creates the value at a dot-delimited path within the given mapping node.
// The value string is parsed as YAML, allowing scalars, sequences, or mappings.
func setYAMLPathValue(root *yaml.Node, dotPath string, raw string) error {
	if root == nil || root.Kind != yaml.MappingNode {
		return fmt.Errorf("root must be a mapping node")
	}
	parts := strings.Split(dotPath, ".")
	cur := root
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return fmt.Errorf("empty path segment in %q", dotPath)
		}
		last := i == len(parts)-1
		// find or create key in cur
		vi := -1
		for j := 0; j+1 < len(cur.Content); j += 2 {
			if cur.Content[j].Kind == yaml.ScalarNode && cur.Content[j].Value == p {
				vi = j + 1
				break
			}
		}
		if last {
			// parse raw into a yaml.Node
			var valDoc yaml.Node
			if err := yaml.Unmarshal([]byte(raw), &valDoc); err != nil || len(valDoc.Content) == 0 {
				// fallback to string scalar
				val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: raw}
				if vi >= 0 {
					cur.Content[vi] = val
					return nil
				}
				// append new key/value pair
				k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: p}
				cur.Content = append(cur.Content, k, val)
				return nil
			}
			val := valDoc.Content[0]
			if vi >= 0 {
				cur.Content[vi] = val
				return nil
			}
			k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: p}
			cur.Content = append(cur.Content, k, val)
			return nil
		}
		// intermediate: ensure mapping node exists and descend
		var next *yaml.Node
		if vi >= 0 {
			next = cur.Content[vi]
			if next.Kind != yaml.MappingNode {
				// convert to mapping
				next.Kind = yaml.MappingNode
				next.Tag = "!!map"
				next.Content = nil
			}
		} else {
			// create key + empty map value
			k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: p}
			v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			cur.Content = append(cur.Content, k, v)
			next = v
		}
		cur = next
	}
	return nil
}
