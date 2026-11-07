package scaffold

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/gitx/gittest"
)

// fakeArchive is a release archive laid out like GoReleaser's: the binary at the top, next to
// README.md. The "binary" is a shell script that echoes its arguments.
func fakeArchive(t *testing.T, say string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name, body string
		mode       int64
	}{
		{"README.md", "# Furrow\n", 0o644},
		{"furrow", "#!/bin/sh\necho \"" + say + " $*\"\n", 0o755},
	} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sum(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

// hostArchive is the archive name the shim picks on this machine, or "" when releases don't cover
// it.
func hostArchive(ver string) string {
	if (runtime.GOOS != "linux" && runtime.GOOS != "darwin") || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return ""
	}
	return fmt.Sprintf("furrow_%s_%s_%s.tar.gz", ver, runtime.GOOS, runtime.GOARCH)
}

// checksumsFor writes a checksums.txt the way GoReleaser does, with hostSum for this machine's
// archive and made-up sums for the other three.
func checksumsFor(ver, hostSum string) []byte {
	var b strings.Builder
	for _, p := range []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"} {
		name := "furrow_" + ver + "_" + p + ".tar.gz"
		s := sum([]byte(name))
		if name == hostArchive(ver) {
			s = hostSum
		}
		fmt.Fprintf(&b, "%s  %s\n", s, name)
	}
	return []byte(b.String())
}

// pinnedRepo runs init and pins version with checksums in a fresh git repo, and returns its root.
func pinnedRepo(t *testing.T, version string, checksums []byte) string {
	t.Helper()
	root := t.TempDir()
	gittest.Must(t, root, "init", "-q", "-b", "main")
	if _, err := Init(root, "APP"); err != nil {
		t.Fatal(err)
	}
	shim, err := ShimText(version, checksums)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WritePin(root, shim); err != nil {
		t.Fatal(err)
	}
	return root
}

// runShim runs bin/furrow with /bin/sh from a subfolder of root, downloading from release.
func runShim(t *testing.T, root, release string, args ...string) (string, string, error) {
	t.Helper()
	sub := filepath.Join(root, "docs")
	cmd := exec.Command("/bin/sh", append([]string{filepath.Join(root, "bin", "furrow")}, args...)...)
	cmd.Dir = sub
	cmd.Env = append(os.Environ(), "FURROW_DOWNLOAD_URL=file://"+release, "TMPDIR="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// AC-4
func TestShimChecksum(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("needs curl, which reads the fake release from a file:// URL")
	}
	const version, ver = "v0.1.0", "0.1.0"
	name := hostArchive(ver)
	if name == "" {
		t.Skipf("no release archive for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	good := fakeArchive(t, "pinned furrow")
	release := t.TempDir()
	if err := os.MkdirAll(filepath.Join(release, version), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(release, version, name), good, 0o644); err != nil {
		t.Fatal(err)
	}

	// The right checksum: the shim downloads, installs into .furrow/bin/, and runs it.
	root := pinnedRepo(t, version, checksumsFor(ver, sum(good)))
	if info, err := os.Stat(filepath.Join(root, "bin", "furrow")); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("bin/furrow should be executable: %v %v", info, err)
	}
	out, errOut, err := runShim(t, root, release, "next", "--json")
	if err != nil {
		t.Fatalf("shim failed: %v\n%s", err, errOut)
	}
	if out != "pinned furrow next --json\n" {
		t.Errorf("shim printed %q", out)
	}
	installed := filepath.Join(".furrow", "bin", version, "furrow")
	if _, err := os.Stat(filepath.Join(root, installed)); err != nil {
		t.Fatalf("binary not installed: %v", err)
	}
	if _, err := gittest.Git(root).Run("check-ignore", "-q", installed); err != nil {
		t.Errorf("%s should be ignored by git: %v", installed, err)
	}
	if _, err := gittest.Git(root).Run("check-ignore", "-q", "bin/furrow"); err == nil {
		t.Errorf("bin/furrow should not be ignored by git")
	}
	// The second run uses the installed copy, even with the release gone.
	if out, errOut, err := runShim(t, root, filepath.Join(release, "gone"), "version"); err != nil || out != "pinned furrow version\n" {
		t.Errorf("second run: %q %q %v", out, errOut, err)
	}

	// A binary whose checksum doesn't match is refused, and nothing is installed.
	for _, c := range []struct {
		label string
		pin   []byte
	}{
		{"tampered release", checksumsFor(ver, sum(good))},
		{"wrong pin", checksumsFor(ver, sum([]byte("something else")))},
	} {
		label, pin := c.label, c.pin
		t.Run(label, func(t *testing.T) {
			release := release
			if label == "tampered release" {
				release = t.TempDir()
				_ = os.MkdirAll(filepath.Join(release, version), 0o755)
				if err := os.WriteFile(filepath.Join(release, version, name), fakeArchive(t, "evil furrow"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			root := pinnedRepo(t, version, pin)
			out, errOut, err := runShim(t, root, release, "next")
			if err == nil {
				t.Fatalf("shim ran a binary with the wrong checksum: %q", out)
			}
			if out != "" || !strings.Contains(errOut, "refused "+name) || !strings.Contains(errOut, "sha256") {
				t.Errorf("stdout %q, stderr %q", out, errOut)
			}
			if _, err := os.Stat(filepath.Join(root, ".furrow", "bin", version)); err == nil {
				t.Errorf("a refused binary was installed")
			}
		})
	}
}

func TestShimText(t *testing.T) {
	good := checksumsFor("0.1.0", sum([]byte("x")))
	shim, err := ShimText("v0.1.0", append([]byte("deadbeef  furrow_0.1.0_windows_amd64.zip\n"), good...))
	if err != nil {
		t.Fatal(err)
	}
	if PinnedVersion(shim) != "v0.1.0" {
		t.Errorf("PinnedVersion = %q", PinnedVersion(shim))
	}
	if !strings.HasPrefix(shim, "#!/bin/sh\n") || strings.Contains(shim, "{{") || strings.Contains(shim, "windows") {
		t.Errorf("shim:\n%s", shim)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(good)), "\n") {
		if !strings.Contains(shim, line+"\n") {
			t.Errorf("shim is missing %q", line)
		}
	}
	for _, c := range []struct{ version, checksums, want string }{
		{"dev", string(good), "isn't a release version"},
		{"0.1.0", string(good), "isn't a release version"},
		{"v0.1.0; rm -rf /", string(good), "isn't a release version"},
		{"v0.2.0", string(good), "no checksum for furrow_0.2.0_darwin_amd64.tar.gz"},
		{"v0.1.0", strings.Replace(string(good), "linux_arm64", "linux_386", 1), "no checksum for furrow_0.1.0_linux_arm64.tar.gz"},
	} {
		if _, err := ShimText(c.version, []byte(c.checksums)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("ShimText(%q): err = %v, want %q", c.version, err, c.want)
		}
	}
}

func TestWritePinKeeps(t *testing.T) {
	root := t.TempDir()
	shim, err := ShimText("v0.1.0", checksumsFor("0.1.0", sum([]byte("x"))))
	if err != nil {
		t.Fatal(err)
	}
	res, err := WritePin(root, shim)
	if err != nil {
		t.Fatal(err)
	}
	var first bytes.Buffer
	res.Text(&first)
	if first.String() != "created bin/furrow\ncreated .furrow/.gitignore\n" {
		t.Errorf("first pin printed %q", first.String())
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "furrow"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err = WritePin(root, shim)
	if err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	res.Text(&second)
	if !strings.HasPrefix(second.String(), "kept bin/furrow\nkept .furrow/.gitignore\n") || !strings.Contains(second.String(), "delete bin/furrow") {
		t.Errorf("second pin printed %q", second.String())
	}
	if got, _ := os.ReadFile(filepath.Join(root, "bin", "furrow")); string(got) != "#!/bin/sh\n" {
		t.Errorf("WritePin overwrote bin/furrow")
	}
}

// tagRemote makes a bare repo with the given tags, standing in for Furrow's own repo.
func tagRemote(t *testing.T, tags ...string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Furrow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	remote := gittest.Init(t, dir)
	for _, tag := range tags {
		gittest.Must(t, dir, "tag", "-a", "-m", tag, tag)
	}
	gittest.Must(t, dir, "push", "-q", "origin", "--tags")
	return remote
}

func TestDoctor(t *testing.T) {
	remote := tagRemote(t, "v0.1.0", "v0.2.0", "v0.10.0", "v0.11.0-rc.1", "not-a-version")
	git := gittest.Git(t.TempDir())
	pin := func(version string) string {
		root := t.TempDir()
		ver := strings.TrimPrefix(version, "v")
		shim, err := ShimText(version, checksumsFor(ver, sum([]byte("x"))))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := WritePin(root, shim); err != nil {
			t.Fatal(err)
		}
		return root
	}

	cases := []struct {
		root   string
		want   PinStatus
		prints string
	}{
		{pin("v0.2.0"), PinStatus{Shim: "bin/furrow", Pinned: "v0.2.0", Latest: "v0.10.0", Behind: true, Copies: []string{}},
			"bin/furrow pins v0.2.0, but v0.10.0 is out. To move the pin, delete bin/furrow and run `furrow init --pin` with furrow v0.10.0.\n"},
		{pin("v0.10.0"), PinStatus{Shim: "bin/furrow", Pinned: "v0.10.0", Latest: "v0.10.0", Copies: []string{}},
			"bin/furrow pins v0.10.0, the latest.\n"},
		{t.TempDir(), PinStatus{Shim: "bin/furrow", Copies: []string{}},
			"No pinned copy: bin/furrow doesn't exist. Run `furrow init --pin` to add one.\n"},
	}
	for _, c := range cases {
		got, err := Doctor(c.root, git, remote)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Doctor = %+v, want %+v", got, c.want)
		}
		var b bytes.Buffer
		got.Text(&b)
		if b.String() != c.prints {
			t.Errorf("printed %q, want %q", b.String(), c.prints)
		}
	}
	data, _ := json.Marshal(cases[0].want)
	if string(data) != `{"shim":"bin/furrow","pinned":"v0.2.0","latest":"v0.10.0","behind":true,"copies":[]}` {
		t.Errorf("JSON = %s", data)
	}

	if _, err := Doctor(pin("v0.1.0"), git, filepath.Join(t.TempDir(), "missing.git")); err == nil || !strings.Contains(err.Error(), "Couldn't read the releases") {
		t.Errorf("unreachable repo: err = %v", err)
	}
	if _, err := Doctor(pin("v0.1.0"), git, tagRemote(t)); err == nil || !strings.Contains(err.Error(), "No releases") {
		t.Errorf("no tags: err = %v", err)
	}
}

// AC-5 (FUR-0020)
func TestDoctorCopies(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{".claude/skills/furrow-swarm", ".claude/skills/furrow-next", ".claude/skills/mine", ".claude/agents"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{".claude/skills/furrow-file", ".claude/agents/furrow-builder.md", ".claude/agents/furrow-verifier.md", ".claude/agents/furrow-notes.txt", ".claude/agents/mine.md"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(f)), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// No bin/furrow and no reachable releases: doctor still reports the copies, with the pin empty.
	git := gittest.Git(t.TempDir())
	got, err := Doctor(root, git, filepath.Join(t.TempDir(), "missing.git"))
	if err != nil {
		t.Fatalf("doctor without bin/furrow should not fail: %v", err)
	}
	want := []string{".claude/agents/furrow-builder.md", ".claude/agents/furrow-verifier.md", ".claude/skills/furrow-next", ".claude/skills/furrow-swarm"}
	if !reflect.DeepEqual(got.Copies, want) {
		t.Errorf("copies = %v, want %v", got.Copies, want)
	}
	if got.Pinned != "" || got.Latest != "" || got.Behind {
		t.Errorf("pin fields should be empty without bin/furrow: %+v", got)
	}
	var b bytes.Buffer
	got.Text(&b)
	wantText := "No pinned copy: bin/furrow doesn't exist. Run `furrow init --pin` to add one.\n" +
		"Copies of Furrow's skills and agents hide the plugin's:\n" +
		"  .claude/agents/furrow-builder.md\n" +
		"  .claude/agents/furrow-verifier.md\n" +
		"  .claude/skills/furrow-next\n" +
		"  .claude/skills/furrow-swarm\n" +
		"Delete them and use the plugin: see \"Use Furrow in your repos\" in Furrow's README.\n"
	if b.String() != wantText {
		t.Errorf("printed\n%s\nwant\n%s", b.String(), wantText)
	}
	data, _ := json.Marshal(got)
	if !strings.Contains(string(data), `"copies":[".claude/agents/furrow-builder.md",".claude/agents/furrow-verifier.md",".claude/skills/furrow-next",".claude/skills/furrow-swarm"]`) {
		t.Errorf("JSON = %s", data)
	}

	// None: an empty list in JSON, and nothing about copies in the text.
	got, err = Doctor(t.TempDir(), git, filepath.Join(t.TempDir(), "missing.git"))
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(got)
	if !strings.Contains(string(data), `"copies":[]`) {
		t.Errorf("JSON with no copies = %s", data)
	}
	b.Reset()
	got.Text(&b)
	if strings.Contains(b.String(), "Copies") {
		t.Errorf("printed copies when there are none: %s", b.String())
	}
}

// AC-5 (FUR-0020): links aren't copies. A .claude/skills or .claude/agents that is itself a link
// into the repo can't drift, and neither can a linked entry.
func TestDoctorCopiesSkipsLinks(t *testing.T) {
	git := gittest.Git(t.TempDir())
	missing := filepath.Join(t.TempDir(), "missing.git")

	// Furrow's own layout: .claude/skills -> ../skills, .claude/agents -> ../agents.
	root := t.TempDir()
	for _, d := range []string{"skills/furrow-swarm", "agents", ".claude"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "agents", "furrow-builder.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, l := range []string{"skills", "agents"} {
		if err := os.Symlink(filepath.Join("..", l), filepath.Join(root, ".claude", l)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Doctor(root, git, missing)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Copies) != 0 {
		t.Errorf("linked .claude/skills and .claude/agents aren't copies, got %v", got.Copies)
	}

	// Real directories holding links next to a real copy: only the copy is listed.
	root = t.TempDir()
	for _, d := range []string{"skills/furrow-next", "agents", ".claude/skills/furrow-swarm", ".claude/agents"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "agents", "furrow-builder.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		".claude/skills/furrow-next":       "../../skills/furrow-next",
		".claude/agents/furrow-builder.md": "../../agents/furrow-builder.md",
	}
	for link, target := range links {
		if err := os.Symlink(filepath.FromSlash(target), filepath.Join(root, filepath.FromSlash(link))); err != nil {
			t.Fatal(err)
		}
	}
	got, err = Doctor(root, git, missing)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{".claude/skills/furrow-swarm"}; !reflect.DeepEqual(got.Copies, want) {
		t.Errorf("copies = %v, want %v", got.Copies, want)
	}
}
