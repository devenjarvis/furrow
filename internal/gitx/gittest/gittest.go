// Package gittest turns a fixture folder into a git repo with a bare remote, for tests that need
// real git. Nothing touches the network.
package gittest

import (
	"path/filepath"
	"testing"

	"github.com/devenjarvis/furrow/internal/gitx"
)

// Env gives tests a fixed identity and keeps the user's git config out of the way.
var Env = []string{
	"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
	"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
}

// Git returns a runner for dir with the test environment.
func Git(dir string) gitx.Git { return gitx.Git{Dir: dir, Env: Env} }

// Must runs git and fails the test on error.
func Must(t testing.TB, dir string, args ...string) string {
	t.Helper()
	out, err := Git(dir).Run(args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Init makes dir a git repo on main with everything committed, creates a bare remote next to it,
// pushes main there as origin, and returns the remote's path.
func Init(t testing.TB, dir string) string {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	Must(t, dir, "init", "-q", "-b", "main")
	Must(t, dir, "add", "-A")
	Must(t, dir, "commit", "-q", "-m", "start")
	Must(t, filepath.Dir(remote), "init", "-q", "--bare", "-b", "main", remote)
	Must(t, dir, "remote", "add", "origin", remote)
	Must(t, dir, "push", "-q", "origin", "main")
	return remote
}

// Clone clones remote into a new temp dir and returns it.
func Clone(t testing.TB, remote string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "clone")
	Must(t, filepath.Dir(dir), "clone", "-q", remote, dir)
	return dir
}

// Commit commits everything in dir with a message.
func Commit(t testing.TB, dir, msg string) {
	t.Helper()
	Must(t, dir, "add", "-A")
	Must(t, dir, "commit", "-q", "-m", msg)
}
