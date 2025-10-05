package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// run executes a command in a directory and returns error, if any.
func run(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com", "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &cmdError{s: string(out)}
	}
	return nil
}

type cmdError struct{ s string }

func (e *cmdError) Error() string { return e.s }

func TestCommitAndPush_LocalRemote(t *testing.T) {
	tmp := t.TempDir()
	work := filepath.Join(tmp, "work")
	remote := filepath.Join(tmp, "remote.git")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	// init bare remote
	if err := run(remote, "git", "init", "--bare"); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	// init work repo
	if err := run(work, "git", "init"); err != nil {
		t.Fatalf("init work: %v", err)
	}
	if err := run(work, "git", "checkout", "-b", "main"); err != nil {
		t.Fatalf("checkout main: %v", err)
	}
	if err := run(work, "git", "remote", "add", "origin", remote); err != nil {
		t.Fatalf("remote add: %v", err)
	}

	// create a file to commit
	f := filepath.Join(work, "file.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set runner that runs git in work dir
	old := runner
	t.Cleanup(func() { runner = old })
	runner = runnerInDir{dir: work}

	// Create an initial commit and push upstream so future pushes work without -u
	if err := run(work, "git", "add", "file.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := run(work, "git", "commit", "-m", "seed"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if err := run(work, "git", "push", "-u", "origin", "main"); err != nil {
		t.Fatalf("git push -u: %v", err)
	}

	// Modify file so there's something to commit via our helper
	if err := os.WriteFile(f, []byte("hello2"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CommitAndPush([]string{"file.txt"}, "update"); err != nil {
		t.Fatalf("commit/push: %v", err)
	}
	// Second call with no changes should succeed (nothing to commit)
	if err := CommitAndPush([]string{"file.txt"}, "again"); err != nil {
		t.Fatalf("second commit should be no-op: %v", err)
	}
}

type runnerInDir struct{ dir string }

func (r runnerInDir) Run(name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", string(out), err
	}
	return string(out), "", nil
}

// fake runner to simulate errors and branches
type fakeRunner struct {
	seq []resp
	i   int
}
type resp struct {
	out, errStr string
	err         error
}

func (f *fakeRunner) Run(name string, args ...string) (string, string, error) {
	if f.i >= len(f.seq) {
		return "", "", nil
	}
	r := f.seq[f.i]
	f.i++
	return r.out, r.errStr, r.err
}

func TestCommitAndPush_ErrorPaths(t *testing.T) {
	old := runner
	defer func() { runner = old }()

	// git add fails
	fr := &fakeRunner{seq: []resp{{"", "boom", &cmdError{s: "add fail"}}}}
	runner = fr
	if err := CommitAndPush([]string{"x"}, "msg"); err == nil {
		t.Fatalf("expected error on add fail")
	}

	// git commit: nothing to commit should be success
	fr = &fakeRunner{seq: []resp{{"", "", nil}, {"", "nothing to commit", &cmdError{s: "commit fail"}}, {"", "", nil}}}
	runner = fr
	if err := CommitAndPush([]string{"x"}, "msg"); err != nil {
		t.Fatalf("expected success on 'nothing to commit': %v", err)
	}

	// git commit: other error should propagate
	fr = &fakeRunner{seq: []resp{{"", "", nil}, {"", "other error", &cmdError{s: "commit fail"}}}}
	runner = fr
	if err := CommitAndPush([]string{"x"}, "msg"); err == nil {
		t.Fatalf("expected error on commit fail")
	}

	// git commit: no changes added to commit -> success
	fr = &fakeRunner{seq: []resp{{"", "", nil}, {"", "no changes added to commit", &cmdError{s: "commit fail"}}, {"", "", nil}}}
	runner = fr
	if err := CommitAndPush([]string{"x"}, "msg"); err != nil {
		t.Fatalf("expected success on 'no changes': %v", err)
	}

	// git push fails
	fr = &fakeRunner{seq: []resp{{"", "", nil}, {"", "", nil}, {"", "push fail", &cmdError{s: "push fail"}}}}
	runner = fr
	if err := CommitAndPush([]string{"x"}, "msg"); err == nil {
		t.Fatalf("expected error on push fail")
	}
}

func TestCommitAndPush_NoPaths(t *testing.T) {
	if err := CommitAndPush(nil, "msg"); err != nil {
		t.Fatalf("expected nil on no paths, got %v", err)
	}
}
