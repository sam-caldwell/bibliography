package gitutil

import (
	"bytes"
	"fmt"
	"os/exec"
)

// Runner abstracts command execution for testability.
type Runner interface {
	Run(name string, args ...string) (stdout string, stderr string, err error)
}

type defaultRunner struct{}

func (defaultRunner) Run(name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

var runner Runner = defaultRunner{}

// SetRunner allows tests to inject a fake runner.
func SetRunner(r Runner) { runner = r }

// CommitAndPush stages the given paths, commits with message, and pushes.
// Treats "nothing to commit" as success.
func CommitAndPush(paths []string, message string) error {
    if len(paths) == 0 {
        return nil
    }
    // git add
    args := append([]string{"add"}, paths...)
    if _, stderr, err := runner.Run("git", args...); err != nil {
        return fmt.Errorf("git add failed: %v: %s", err, stderr)
    }
    // git commit
    if _, stderr, err := runner.Run("git", "commit", "-m", message); err != nil {
        // If nothing to commit, treat as success
        if bytes.Contains([]byte(stderr), []byte("nothing to commit")) ||
            bytes.Contains([]byte(stderr), []byte("no changes added to commit")) {
            // no-op
        } else {
            return fmt.Errorf("git commit failed: %v: %s", err, stderr)
        }
    }
    // git push
    if _, stderr, err := runner.Run("git", "push"); err != nil {
        // If no upstream configured, try to set upstream to origin <current-branch>
        if bytes.Contains([]byte(stderr), []byte("has no upstream branch")) ||
            bytes.Contains([]byte(stderr), []byte("no configured push destination")) {
            br, _, berr := runner.Run("git", "rev-parse", "--abbrev-ref", "HEAD")
            branch := "HEAD"
            if berr == nil && br != "" {
                branch = string(bytes.TrimSpace([]byte(br)))
            }
            if _, stderr2, err2 := runner.Run("git", "push", "-u", "origin", branch); err2 != nil {
                return fmt.Errorf("git push failed: %v: %s; fallback failed: %v: %s", err, stderr, err2, stderr2)
            }
        } else {
            return fmt.Errorf("git push failed: %v: %s", err, stderr)
        }
    }
    return nil
}
