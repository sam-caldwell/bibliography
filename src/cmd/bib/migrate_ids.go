package main

import (
	"crypto/rand"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"bibliography/src/internal/gitutil"
)

// newMigrateIDsCmd migrates citation YAML files to UUIDv4 ids and renames files accordingly.
func newMigrateIDsCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "migrate-ids",
		Short: "Migrate citation YAML ids to UUIDv4 and rename files",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "data/citations"
			var changed []string
			var renamed []string
			reUUIDv4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
			// Walk all yaml files
			err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".yaml") {
					return nil
				}
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				// parse minimal mapping
				var root yaml.Node
				if err := yaml.Unmarshal(b, &root); err != nil {
					return nil
				}
				if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
					return nil
				}
				m := root.Content[0]
				if m.Kind != yaml.MappingNode {
					return nil
				}
				// find id and type
				var idNode, typeNode *yaml.Node
				for i := 0; i+1 < len(m.Content); i += 2 {
					if m.Content[i].Value == "id" {
						idNode = m.Content[i+1]
					}
					if m.Content[i].Value == "type" {
						typeNode = m.Content[i+1]
					}
				}
				if idNode == nil || typeNode == nil {
					return nil
				}
				oldID := strings.TrimSpace(idNode.Value)
				if reUUIDv4.MatchString(strings.ToLower(oldID)) {
					return nil // already migrated
				}
				// generate new id
				newID := uuidv4()
				// path for rename
				dirp := filepath.Dir(path)
				newPath := filepath.Join(dirp, newID+".yaml")
				changed = append(changed, path)

				if dryRun {
					fmt.Fprintf(cmd.OutOrStdout(), "would update id in %s: %q -> %q\n", path, oldID, newID)
					if path != newPath {
						fmt.Fprintf(cmd.OutOrStdout(), "would rename %s -> %s\n", path, newPath)
					}
					return nil
				}

				// write updated yaml
				idNode.Value = newID
				out, err := yaml.Marshal(&root)
				if err != nil {
					return err
				}
				if err := os.WriteFile(path, out, 0o644); err != nil {
					return err
				}
				// rename file to match new id
				if path != newPath {
					if err := os.Rename(path, newPath); err == nil {
						renamed = append(renamed, newPath)
						fmt.Fprintf(cmd.OutOrStdout(), "renamed %s -> %s\n", path, newPath)
					}
				}
				return nil
			})
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if len(changed) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no ids needed migration")
				return nil
			}
			if dryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: %d files would be migrated\n", len(changed))
				return nil
			}
			if err := gitutil.CommitAndPush([]string{"data/citations"}, "migrate: convert citation ids to UUIDv4"); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "migrated %d files to UUIDv4 ids\n", len(changed))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show planned changes without writing or committing")
	return cmd
}

// local uuid v4 generator (canonical form) for migration without depending on schema package
func uuidv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	const hexd = "0123456789abcdef"
	toHex := func(x byte) (byte, byte) { return hexd[x>>4], hexd[x&0x0f] }
	out := make([]byte, 36)
	pos := 0
	write := func(c byte) { out[pos] = c; pos++ }
	wh := func(x byte) { a, b := toHex(x); write(a); write(b) }
	for i, v := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			write('-')
		}
		wh(v)
	}
	return string(out)
}
