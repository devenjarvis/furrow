package claim

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/schedule"
	"github.com/devenjarvis/furrow/internal/spec"
)

// ReleaseTrailer marks a release commit; its value is a nonce. A claim branch whose tip is a
// release commit is released: furrow treats its step as unclaimed.
const ReleaseTrailer = "furrow-release"

var nonceRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

// releaseMessage reports whether msg is a release commit's message for ref: its subject is
// "Release <ref>" and its last paragraph has a furrow-release: trailer with a 16-hex nonce. A
// line elsewhere that merely mentions the trailer doesn't count.
func releaseMessage(msg string, ref spec.Ref) bool {
	paras := strings.Split(strings.TrimSpace(strings.ReplaceAll(msg, "\r\n", "\n")), "\n\n")
	if len(paras) < 2 || strings.TrimSpace(paras[0]) != "Release "+ref.String() {
		return false
	}
	for _, line := range strings.Split(paras[len(paras)-1], "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), ReleaseTrailer+":"); ok && nonceRe.MatchString(strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}

// IsRelease reports whether rev is a release commit for ref, as the spec defines one: exactly one
// parent, the same tree as that parent, the subject "Release <ref>", and a furrow-release: trailer
// with a 16-hex nonce in the message's last paragraph. Only a branch whose tip passes is released.
func IsRelease(g gitx.Git, rev string, ref spec.Ref) (bool, error) {
	out, err := g.Run("log", "-1", "--format=%P%x00%T%x00%B", rev)
	if err != nil {
		return false, err
	}
	parts := strings.SplitN(out, "\x00", 3)
	if len(parts) != 3 || !releaseMessage(parts[2], ref) {
		return false, nil
	}
	parents := strings.Fields(parts[0])
	if len(parents) != 1 {
		return false, nil
	}
	tree, err := g.Run("rev-parse", parents[0]+"^{tree}")
	if err != nil {
		return false, err
	}
	return tree == parts[1], nil
}

// Released is how furrow release gave a step back. How is "deleted" when the branch was deleted,
// "marked" when the remote refused and furrow pushed a release commit, and "" when there was
// nothing to release. WasReleased says the branch was already released.
type Released struct {
	Ref         string `json:"ref"`
	Branch      string `json:"branch"`
	Released    bool   `json:"released"`
	How         string `json:"how"`
	WasReleased bool   `json:"-"`
}

// Text writes a release as furrow release prints it.
func (r *Released) Text(w io.Writer, remote string) {
	switch {
	case r.How == "deleted":
		fmt.Fprintf(w, "Released %s: deleted %s on %s.\n", r.Ref, r.Branch, remote)
	case r.How == "marked":
		fmt.Fprintf(w, "Released %s: %s wouldn't delete %s, so furrow marked it released.\n", r.Ref, remote, r.Branch)
		fmt.Fprintln(w, "Delete the branch when you can; until then furrow treats it as free.")
	case r.WasReleased:
		fmt.Fprintf(w, "%s isn't claimed (%s on %s is released).\n", r.Ref, r.Branch, remote)
	default:
		fmt.Fprintf(w, "%s isn't claimed (no %s on %s).\n", r.Ref, r.Branch, remote)
	}
}

// Release gives a step back. It deletes the claim branch, and when the remote refuses the delete
// it pushes a release commit on the tip ls-remote reported instead, leased on that tip, so the
// push is a fast-forward a remote that refuses deletes and force-pushes still takes. Releasing an
// unclaimed or already-released step does nothing.
func Release(g gitx.Git, repo *spec.Repo, text string) (*Released, error) {
	st, err := repo.StepRef(text)
	if err != nil {
		return nil, err
	}
	cfg := repo.Config
	ref := st.Ref().String()
	branch := Branch(cfg, st.Ref())
	res := &Released{Ref: ref, Branch: branch}
	out, err := g.Run("ls-remote", "--heads", cfg.Remote, "refs/heads/"+branch)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrRemote, cfg.Remote, err)
	}
	tip, _, _ := strings.Cut(out, "\t")
	if tip == "" {
		return res, nil
	}
	if err := FetchClaims(g, cfg, []schedule.Claim{{Branch: branch}}); err != nil {
		return nil, err
	}
	moved := fmt.Errorf("%s moved while furrow was releasing it. Run furrow release %s again.", branch, ref)
	if fetched, err := g.Run("rev-parse", ClaimsRef(branch)); err != nil || fetched != tip {
		return nil, moved
	}
	was, err := IsRelease(g, tip, st.Ref())
	if err != nil {
		return nil, err
	}
	if was {
		res.WasReleased = true
		return res, nil
	}

	if _, err := g.Run("push", "-q", cfg.Remote, "--delete", "refs/heads/"+branch); err == nil {
		res.Released, res.How = true, "deleted"
		return res, nil
	}
	nonce := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("Release %s\n\n%s: %s", ref, ReleaseTrailer, hex.EncodeToString(nonce))
	sha, err := g.Run("commit-tree", tip+"^{tree}", "-p", tip, "-m", msg)
	if err != nil {
		return nil, err
	}
	if _, err := g.Run("push", "-q", cfg.Remote, sha+":refs/heads/"+branch, "--force-with-lease=refs/heads/"+branch+":"+tip); err != nil {
		var gerr *gitx.Error
		if errors.As(err, &gerr) && (strings.Contains(gerr.Stderr, "stale info") || strings.Contains(gerr.Stderr, "but expected")) {
			return nil, moved
		}
		return nil, fmt.Errorf("%w %s: %v", ErrRemote, cfg.Remote, err)
	}
	res.Released, res.How = true, "marked"
	return res, nil
}
