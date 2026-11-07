package scaffold

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/devenjarvis/furrow/internal/gitx"
)

// ShimPath is where furrow init --pin writes the pinned shim, from the repo root.
const ShimPath = "bin/furrow"

// ReleaseRepo is Furrow's own repo. Its v* tags are the releases (.goreleaser.yaml).
const ReleaseRepo = "https://github.com/devenjarvis/furrow.git"

// Platforms are the release archives' os_arch suffixes, as .goreleaser.yaml builds them.
var Platforms = []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"}

var (
	versionRe  = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z.-]+)?$`)
	checksumRe = regexp.MustCompile(`^([0-9a-f]{64})\s+\*?(\S+)$`)
	pinLineRe  = regexp.MustCompile(`(?m)^FURROW_VERSION=(\S+)$`)
)

// ShimText fills in the bin/furrow shell script for version (a release tag such as v0.1.0) from
// that release's checksums.txt. It keeps the four archives' lines and refuses a file missing one.
func ShimText(version string, checksums []byte) (string, error) {
	if !versionRe.MatchString(version) {
		return "", fmt.Errorf("%q isn't a release version. Pin a release tag such as v0.1.0 with --version.", version)
	}
	ver := strings.TrimPrefix(version, "v")
	sums := map[string]string{}
	for _, l := range strings.Split(strings.ReplaceAll(string(checksums), "\r\n", "\n"), "\n") {
		if m := checksumRe.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			sums[m[2]] = m[1]
		}
	}
	var lines []string
	for _, p := range Platforms {
		name := "furrow_" + ver + "_" + p + ".tar.gz"
		s, ok := sums[name]
		if !ok {
			return "", fmt.Errorf("The checksums have no checksum for %s. Pass the checksums.txt from the %s release page.", name, version)
		}
		lines = append(lines, s+"  "+name)
	}
	sort.Strings(lines)
	return strings.NewReplacer("{{VERSION}}", version, "{{CHECKSUMS}}", strings.Join(lines, "\n")+"\n").Replace(asset("furrow.sh")), nil
}

// ChecksumsURL is where a release publishes its checksums.txt.
func ChecksumsURL(version string) string {
	return "https://github.com/devenjarvis/furrow/releases/download/" + version + "/checksums.txt"
}

// WritePin writes the shim to bin/furrow, executable, and .furrow/.gitignore so the binaries the
// shim downloads stay out of git. Like init, it never overwrites a file.
func WritePin(root, shim string) (InitResult, error) {
	var res InitResult
	for _, f := range []struct {
		rel, content string
		mode         os.FileMode
	}{
		{ShimPath, shim, 0o755},
		{".furrow/.gitignore", "# Furrow downloads its pinned binary here (bin/furrow). Keep it out of git.\n/bin/\n", 0o644},
	} {
		path := filepath.Join(root, filepath.FromSlash(f.rel))
		created, err := writeNew(path, f.content)
		if err != nil {
			return res, err
		}
		if created && f.mode != 0o644 {
			if err := os.Chmod(path, f.mode); err != nil {
				return res, err
			}
		}
		res.Entries = append(res.Entries, Entry{Path: f.rel, Created: created})
	}
	if !res.Entries[0].Created {
		res.Notes = append(res.Notes, "bin/furrow exists, so the pin didn't change. To move it, delete bin/furrow and run `furrow init --pin` again.")
	}
	return res, nil
}

// PinnedVersion reads the version a shim pins, or "" if it isn't a Furrow shim.
func PinnedVersion(shim string) string {
	if m := pinLineRe.FindStringSubmatch(shim); m != nil && versionRe.MatchString(m[1]) {
		return m[1]
	}
	return ""
}

// PinStatus is what furrow doctor found: the pinned version, the latest release, whether the
// pin is behind it, and any copies of Furrow's skills and agents. Pinned and Latest are "" when
// the repo has no shim.
type PinStatus struct {
	Shim   string   `json:"shim"`
	Pinned string   `json:"pinned"`
	Latest string   `json:"latest"`
	Behind bool     `json:"behind"`
	Copies []string `json:"copies"`
}

// Text says where the pin stands and what to do next.
func (s PinStatus) Text(w io.Writer) {
	switch {
	case s.Pinned == "":
		fmt.Fprintf(w, "No pinned copy: %s doesn't exist. Run `furrow init --pin` to add one.\n", s.Shim)
	case s.Behind:
		fmt.Fprintf(w, "%s pins %s, but %s is out. To move the pin, delete %s and run `furrow init --pin` with furrow %s.\n", s.Shim, s.Pinned, s.Latest, s.Shim, s.Latest)
	case s.Pinned == s.Latest:
		fmt.Fprintf(w, "%s pins %s, the latest.\n", s.Shim, s.Pinned)
	default:
		fmt.Fprintf(w, "%s pins %s. The latest release is %s.\n", s.Shim, s.Pinned, s.Latest)
	}
	if len(s.Copies) > 0 {
		fmt.Fprintln(w, "Copies of Furrow's skills and agents hide the plugin's:")
		for _, c := range s.Copies {
			fmt.Fprintf(w, "  %s\n", c)
		}
		fmt.Fprintln(w, `Delete them and use the plugin: see "Use Furrow in your repos" in Furrow's README.`)
	}
}

// Doctor lists copies of Furrow's skills and agents and compares the repo's pinned version with
// the latest release tag in repo, read with `git ls-remote`, so it needs only git (INV-2, INV-7).
// Without a shim it doesn't read the releases, so it works offline.
func Doctor(root string, git gitx.Git, repo string) (PinStatus, error) {
	st := PinStatus{Shim: ShimPath}
	var err error
	if st.Copies, err = FindCopies(root); err != nil {
		return st, err
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ShimPath)))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return st, nil
	case err != nil:
		return st, err
	default:
		if st.Pinned = PinnedVersion(string(data)); st.Pinned == "" {
			return st, fmt.Errorf("%s isn't a Furrow shim. Move it aside and run `furrow init --pin`.", ShimPath)
		}
	}
	if st.Latest, err = LatestRelease(git, repo); err != nil {
		return st, err
	}
	st.Behind = st.Pinned != "" && compareVersions(st.Pinned, st.Latest) < 0
	return st, nil
}

// LatestRelease is the highest v* tag in repo that isn't a pre-release, or the highest
// pre-release when there's nothing else.
func LatestRelease(git gitx.Git, repo string) (string, error) {
	lines, err := git.Lines("ls-remote", "--tags", "--refs", repo, "refs/tags/v*")
	if err != nil {
		return "", fmt.Errorf("Couldn't read the releases from %s: %v. Check the network, or pass --repo.", repo, err)
	}
	var stable, pre []string
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) != 2 {
			continue
		}
		tag := strings.TrimPrefix(f[1], "refs/tags/")
		if m := versionRe.FindStringSubmatch(tag); m != nil {
			if m[4] == "" {
				stable = append(stable, tag)
			} else {
				pre = append(pre, tag)
			}
		}
	}
	if len(stable) == 0 {
		stable = pre
	}
	if len(stable) == 0 {
		return "", fmt.Errorf("No releases at %s yet: it has no v* tags.", repo)
	}
	sort.Slice(stable, func(i, j int) bool { return compareVersions(stable[i], stable[j]) < 0 })
	return stable[len(stable)-1], nil
}

// compareVersions orders release tags by number; a pre-release comes before its release, and
// pre-releases of one version order by text.
func compareVersions(a, b string) int {
	ma, mb := versionRe.FindStringSubmatch(a), versionRe.FindStringSubmatch(b)
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(ma[i])
		y, _ := strconv.Atoi(mb[i])
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case ma[4] == mb[4]:
		return 0
	case ma[4] == "":
		return 1
	case mb[4] == "":
		return -1
	default:
		return strings.Compare(ma[4], mb[4])
	}
}
