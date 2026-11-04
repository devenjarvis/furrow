package claim

import (
	"fmt"
	"strings"

	"github.com/devenjarvis/furrow/internal/config"
	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/schedule"
	"github.com/devenjarvis/furrow/internal/spec"
)

// ClaimTrailer marks a claim commit; its value is the claim's nonce.
const ClaimTrailer = "furrow-claim"

// StackTrailer names the branch a stacked claim commit was made on.
const StackTrailer = "furrow-stack"

// ClaimsRef is where a claim branch is fetched to read it: refs/furrow/claims/<branch>.
func ClaimsRef(branch string) string { return "refs/furrow/claims/" + branch }

// FetchClaims fetches every claim branch into refs/furrow/claims/, as furrow claims does, so its
// tip can be read without checking it out.
func FetchClaims(g gitx.Git, cfg config.Config, claims []schedule.Claim) error {
	if len(claims) == 0 {
		return nil
	}
	args := []string{"fetch", "-q", "--no-tags", cfg.Remote}
	for _, c := range claims {
		args = append(args, "+refs/heads/"+c.Branch+":"+ClaimsRef(c.Branch))
	}
	if _, err := g.Run(args...); err != nil {
		return fmt.Errorf("%w %s: %v", ErrRemote, cfg.Remote, err)
	}
	return nil
}

// HandedBack reports whether rev's copy of the step's spec file has the step marked merged: true,
// which is what furrow done writes. A rev without the file, or with a copy that doesn't parse or
// doesn't have the step, isn't handed back.
func HandedBack(g gitx.Git, st *spec.Step, rev string) bool {
	text, err := g.Run("show", rev+":"+st.Spec.File)
	if err != nil {
		return false
	}
	s, err := spec.ParseSpec([]byte(text))
	if err != nil || s.ID != st.Spec.ID {
		return false
	}
	for _, other := range s.Steps {
		if other.N == st.N {
			return other.Merged
		}
	}
	return false
}

// ClaimCommit finds a step's claim commit in rev's history: the newest commit whose message starts
// "Claim <ref>:" and has a furrow-claim: trailer. It returns the commit and the branch its
// furrow-stack: trailer names, or "" when it isn't stacked. sha is "" when there's no claim commit.
func ClaimCommit(g gitx.Git, rev string, ref spec.Ref) (sha, stackOn string, err error) {
	out, err := g.Run("log", "--format=%H%x00%B%x1e", "--grep="+ClaimTrailer+":", "--fixed-strings", rev)
	if err != nil {
		return "", "", err
	}
	prefix := "Claim " + ref.String() + ":"
	for _, rec := range strings.Split(out, "\x1e") {
		h, msg, ok := strings.Cut(strings.TrimSpace(rec), "\x00")
		if !ok || !strings.HasPrefix(msg, prefix) {
			continue
		}
		if _, ok := trailer(msg, ClaimTrailer); !ok {
			continue
		}
		stack, _ := trailer(msg, StackTrailer)
		return h, stack, nil
	}
	return "", "", nil
}

// trailer returns the value of a "key: value" line in a commit message.
func trailer(msg, key string) (string, bool) {
	for _, line := range strings.Split(msg, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), key+":"); ok {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

// ReadStack fetches the claim branches and fills in what stacking needs on each claim: whether
// its tip has the step handed back, and the branch its claim commit is stacked on. Claims for
// steps the repo doesn't know are returned as they are. It returns ErrRemote when the fetch fails.
func ReadStack(g gitx.Git, repo *spec.Repo, claims []schedule.Claim) ([]schedule.Claim, error) {
	if err := FetchClaims(g, repo.Config, claims); err != nil {
		return nil, err
	}
	out := make([]schedule.Claim, len(claims))
	for i, c := range claims {
		out[i] = c
		st := repo.Step(c.Ref)
		if st == nil {
			continue
		}
		tip := ClaimsRef(c.Branch)
		out[i].HandedBack = HandedBack(g, st, tip)
		_, stackOn, err := ClaimCommit(g, tip, c.Ref)
		if err != nil {
			return nil, err
		}
		out[i].StackOn = stackOn
	}
	return out, nil
}
