package gitutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Runner abstracts command execution for testability.
type Runner interface {
	Run(name string, args ...string) (stdout string, stderr string, err error)
}

type defaultRunner struct{}

// Run executes the named program with args and returns stdout, stderr, and error.
func (defaultRunner) Run(name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	var out, errB bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errB
	err := cmd.Run()
	return out.String(), errB.String(), err
}

var runner Runner = defaultRunner{}

// CommitAndPush stages the given paths, commits with message, and pushes.
// Treats "nothing to commit" as success.
func CommitAndPush(paths []string, message string) error {
	if len(paths) == 0 {
		return nil
	}
	if err := gitAdd(paths); err != nil {
		return err
	}
	if noChange, err := gitCommit(message); err != nil && !noChange {
		return err
	}
	return gitPushWithFallback()
}

// gitAdd stages additions, modifications, and deletions for the provided paths.
func gitAdd(paths []string) error {
	args := append([]string{"add", "-A"}, paths...)
	if _, stderr, err := runner.Run("git", args...); err != nil {
		return fmt.Errorf("git add failed: %v: %s", err, stderr)
	}
	return nil
}

// gitCommit attempts to create a commit. It returns (noChange=true) when there
// is nothing to commit, which callers treat as success.
func gitCommit(message string) (noChange bool, err error) {
	stdout, stderr, runErr := runner.Run("git", "commit", "-m", message)
	if runErr == nil {
		return false, nil
	}
	// Some Git versions indicate no-op on stdout or stderr; treat as noChange.
	combined := append([]byte(stderr), []byte(stdout)...)
	if bytes.Contains(combined, []byte("nothing to commit")) ||
		bytes.Contains(combined, []byte("no changes added to commit")) ||
		bytes.Contains(combined, []byte("working tree clean")) {
		return true, nil
	}
	return false, fmt.Errorf("git commit failed: %v: %s%s", runErr, stderr, stdout)
}

// gitPushWithFallback runs `git push`, and when there is no upstream configured,
// it falls back to `git push -u origin <current-branch>`.
func gitPushWithFallback() error {
	if _, stderr, err := runner.Run("git", "push"); err != nil {
		// Detect missing upstream situations
		if bytes.Contains([]byte(stderr), []byte("has no upstream branch")) ||
			bytes.Contains([]byte(stderr), []byte("no configured push destination")) {
			// Determine current branch
			br, _, bErr := runner.Run("git", "rev-parse", "--abbrev-ref", "HEAD")
			branch := "HEAD"
			if bErr == nil && strings.TrimSpace(br) != "" {
				branch = strings.TrimSpace(br)
			}
			if _, stderr2, err2 := runner.Run("git", "push", "-u", "origin", branch); err2 != nil {
				return fmt.Errorf("git push failed: %v: %s; fallback failed: %v: %s", err, stderr, err2, stderr2)
			}
			return nil
		}
		return fmt.Errorf("git push failed: %v: %s", err, stderr)
	}
	return nil
}
