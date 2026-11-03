// Package lifecycle hands steps back, lists and records sign-offs, and archives finished specs:
// furrow done, review, signoff and archive. Every command edits a spec file's text in place,
// changing only the line it means to (see edit.go), and never a criterion's text (INV-4).
package lifecycle

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/spec"
)

const dateLayout = "2006-01-02"

// AgentRefusal is what signoff says inside an agent session (INV-3).
const AgentRefusal = "Sign-off is for a person (INV-3). Ask the owner to run this."

// agentVars mark an agent session. Claude Code sets CLAUDECODE; other agents set FURROW_AGENT.
var agentVars = []string{"CLAUDECODE", "FURROW_AGENT"}

func readFile(repo *spec.Repo, rel string) (string, error) {
	data, err := os.ReadFile(filepath.Join(repo.Root, filepath.FromSlash(rel)))
	return string(data), err
}

func writeFile(repo *spec.Repo, rel, text string) error {
	return os.WriteFile(filepath.Join(repo.Root, filepath.FromSlash(rel)), []byte(text), 0o644)
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// and joins ids as "AC-1", "AC-1 and AC-2" or "AC-1, AC-2 and AC-3".
func and(ids []string) string {
	if len(ids) <= 1 {
		return strings.Join(ids, "")
	}
	return strings.Join(ids[:len(ids)-1], ", ") + " and " + ids[len(ids)-1]
}

// Done sets merged: true on the step ref names. It refuses when an after isn't merged, or when a
// criterion the step covers, other than a human: one, is unticked.
func Done(repo *spec.Repo, ref string) (string, error) {
	st, err := repo.StepRef(ref)
	if err != nil {
		return "", err
	}
	s := st.Spec
	if spec.StepMerged(st) {
		return fmt.Sprintf("%s is already merged. Nothing to do.", st.Ref()), nil
	}
	var reasons []string
	refs, errs := repo.AfterRefs(st)
	for _, e := range errs {
		reasons = append(reasons, e.Error())
	}
	for _, a := range refs {
		if !repo.RefMerged(a) {
			reasons = append(reasons, fmt.Sprintf("%s is after %s, which isn't merged. Merge it first.", st.Ref(), a))
		}
	}
	var unticked []string
	for _, id := range st.Covers {
		c := s.Criterion(id)
		switch {
		case c == nil:
			reasons = append(reasons, fmt.Sprintf("%s covers %s, which isn't a criterion. Run `furrow lint`.", st.Ref(), id))
		case !c.Ticked && c.Proof != "human":
			unticked = append(unticked, id)
		}
	}
	switch len(unticked) {
	case 0:
	case 1:
		reasons = append(reasons, fmt.Sprintf("Tick %s with its evidence first.", unticked[0]))
	default:
		reasons = append(reasons, fmt.Sprintf("Tick %s with their evidence first.", and(unticked)))
	}
	if len(reasons) > 0 {
		return "", errors.New(strings.Join(reasons, "\n"))
	}

	text, err := readFile(repo, s.File)
	if err != nil {
		return "", err
	}
	text, err = setMerged(text, st.N)
	if err != nil {
		return "", fmt.Errorf("%s: %v", s.File, err)
	}
	if err := writeFile(repo, s.File, text); err != nil {
		return "", err
	}

	msg := []string{fmt.Sprintf("Marked %s merged in %s.", st.Ref(), s.File)}
	if after, err := spec.ParseSpec([]byte(text)); err == nil {
		var waits []string
		for _, c := range after.Criteria {
			if waiting(after, c) {
				waits = append(waits, c.ID)
			}
		}
		switch {
		case after.Complete():
			msg = append(msg, fmt.Sprintf("Every step is merged and every box ticked: run `furrow archive %s` after this merges.", s.ID))
		case len(waits) > 0:
			msg = append(msg, fmt.Sprintf("%s waits for sign-off: the owner runs `furrow review`.", and(waits)))
		}
	}
	return strings.Join(msg, "\n"), nil
}

// waiting reports whether a criterion is a human: one that's unticked and whose covering steps are
// all merged: what review lists and signoff accepts.
func waiting(s *spec.Spec, c *spec.Criterion) bool {
	return !s.Archived && !c.Ticked && c.Proof == "human" && notWaitingWhy(s, c) == ""
}

// notWaitingWhy says why a human: criterion isn't ready for sign-off, or "" when it is.
func notWaitingWhy(s *spec.Spec, c *spec.Criterion) string {
	covering := 0
	for _, st := range s.Steps {
		if !has(st.Covers, c.ID) {
			continue
		}
		covering++
		if !spec.StepMerged(st) {
			return fmt.Sprintf("step %d isn't merged", st.N)
		}
	}
	if covering == 0 {
		return "no step covers it"
	}
	return ""
}

// Report is what furrow review shows: criteria waiting for a person, grouped by spec.
type Report struct {
	Specs []ReviewSpec `json:"specs"`
}

// ReviewSpec is one spec with criteria waiting for sign-off.
type ReviewSpec struct {
	ID       string           `json:"id"`
	Title    string           `json:"title"`
	File     string           `json:"file"`
	Criteria []ReviewCriteria `json:"criteria"`
}

// ReviewCriteria is one human: criterion waiting for sign-off. LookAt is its proof text.
type ReviewCriteria struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	LookAt string `json:"look_at"`
}

// Review lists every unticked human: criterion whose covering steps are all merged.
func Review(repo *spec.Repo) Report {
	rep := Report{Specs: []ReviewSpec{}}
	for _, s := range repo.Specs {
		var items []ReviewCriteria
		for _, c := range s.Criteria {
			if waiting(s, c) {
				items = append(items, ReviewCriteria{ID: c.ID, Text: criterionText(c), LookAt: c.Detail})
			}
		}
		if len(items) > 0 {
			rep.Specs = append(rep.Specs, ReviewSpec{ID: s.ID, Title: s.Title, File: s.File, Criteria: items})
		}
	}
	return rep
}

// criterionText is the criterion's text without its "(human: …)" proof.
func criterionText(c *spec.Criterion) string {
	if i := strings.LastIndex(c.Text, "("+c.Proof+":"); i >= 0 {
		return strings.TrimSpace(c.Text[:i])
	}
	return c.Text
}

// Text prints the report for a person.
func (r Report) Text(w io.Writer) {
	if len(r.Specs) == 0 {
		fmt.Fprintln(w, "Nothing waiting for sign-off.")
		return
	}
	for _, s := range r.Specs {
		fmt.Fprintf(w, "%s: %s\n", s.ID, s.Title)
		for _, c := range s.Criteria {
			fmt.Fprintf(w, "  %s: %s\n", c.ID, c.Text)
			fmt.Fprintf(w, "        Look at: %s\n", c.LookAt)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "When you've checked one, run `furrow signoff <id> <AC-n>...`.")
}

// CheckPerson refuses inside an agent session (INV-3). A nil getenv counts as an agent.
func CheckPerson(getenv func(string) string) error {
	if getenv == nil {
		return errors.New(AgentRefusal)
	}
	for _, k := range agentVars {
		if getenv(k) != "" {
			return errors.New(AgentRefusal)
		}
	}
	return nil
}

// SignoffRequest is one furrow signoff. By is the person's name, from the --by flag. When it's
// empty, Name supplies it; a nil Name reads `git config user.name` in the repo. Name is only
// called once Getenv shows a person (INV-3). Getenv is how signoff tells a person from an agent.
type SignoffRequest struct {
	ID       string
	Criteria []string
	By       string
	Name     func() string
	Getenv   func(string) string
	Now      time.Time
}

// GitUserName is `git config user.name` in dir, or "" when git has none or dir isn't a repo.
func GitUserName(dir string) string {
	name, err := gitx.Git{Dir: dir}.Run("config", "user.name")
	if err != nil {
		return ""
	}
	return name
}

// Signoff ticks human: criteria that review lists, logs each in ## Decisions, and archives the
// spec when that completes it. It refuses inside an agent session, and changes nothing on any
// refusal.
func Signoff(repo *spec.Repo, req SignoffRequest) (string, error) {
	if err := CheckPerson(req.Getenv); err != nil {
		return "", err
	}
	by := strings.TrimSpace(req.By)
	if by == "" {
		name := req.Name
		if name == nil {
			name = func() string { return GitUserName(repo.Root) }
		}
		by = strings.TrimSpace(name())
	}
	if by == "" {
		return "", errors.New("Say who checked it: --by NAME.")
	}
	if strings.ContainsAny(by, "\r\n") {
		return "", errors.New("--by must be one line.")
	}
	s, err := wholeSpec(repo, req.ID)
	if err != nil {
		return "", err
	}
	if len(req.Criteria) == 0 {
		return "", fmt.Errorf("Name at least one criterion, such as `furrow signoff %s AC-1`.", s.ID)
	}
	var picked []*spec.Criterion
	seen := map[string]bool{}
	for _, id := range req.Criteria {
		if seen[id] {
			continue
		}
		seen[id] = true
		c := s.Criterion(id)
		switch {
		case c == nil:
			return "", fmt.Errorf("%s has no %s.", s.ID, id)
		case c.Proof != "human":
			return "", fmt.Errorf("%s isn't a human: criterion. Agents tick it with its evidence.", id)
		case s.Archived:
			return "", fmt.Errorf("%s is archived. Nothing to sign off.", s.ID)
		case c.Ticked:
			return "", fmt.Errorf("%s is already ticked.", id)
		}
		if why := notWaitingWhy(s, c); why != "" {
			return "", fmt.Errorf("%s isn't waiting for sign-off: %s.", id, why)
		}
		picked = append(picked, c)
	}
	sort.SliceStable(picked, func(i, j int) bool { return picked[i].Num < picked[j].Num })

	text, err := readFile(repo, s.File)
	if err != nil {
		return "", err
	}
	date := req.Now.Format(dateLayout)
	var ids []string
	for _, c := range picked {
		if text, err = tickBox(text, c.ID); err != nil {
			return "", fmt.Errorf("%s: %v", s.File, err)
		}
		text = appendToSection(text, "Decisions", fmt.Sprintf("- %s: %s signed off by %s.", date, c.ID, by))
		ids = append(ids, c.ID)
	}
	after, err := spec.ParseSpec([]byte(text))
	if err != nil {
		return "", fmt.Errorf("%s: %v", s.File, err)
	}
	msg := fmt.Sprintf("Signed off %s on %s by %s.", and(ids), s.ID, by)
	if !after.Complete() {
		if err := writeFile(repo, s.File, text); err != nil {
			return "", err
		}
		return msg, nil
	}
	plan, err := planArchive(repo, s, text, req.Now)
	if err != nil {
		if werr := writeFile(repo, s.File, text); werr != nil {
			return "", werr
		}
		return "", fmt.Errorf("%s\nCan't archive it yet: %v", msg, err)
	}
	if err := plan.apply(repo); err != nil {
		return "", err
	}
	return msg + "\n" + plan.message(), nil
}

// wholeSpec finds the spec a ref like FUR-0005 names.
func wholeSpec(repo *spec.Repo, text string) (*spec.Spec, error) {
	ref, err := repo.ParseRef(text, nil)
	if err != nil {
		return nil, err
	}
	if ref.Step != 0 {
		return nil, fmt.Errorf("%s names a step; name the spec, such as %s", ref, ref.Spec)
	}
	s := repo.Spec(ref.Spec)
	if s == nil {
		return nil, fmt.Errorf("no spec %s", ref.Spec)
	}
	return s, nil
}

// Archive sets status: done, moves the spec to archive/, and adds it to the area's ## History. It
// refuses unless every step is merged and every box ticked.
func Archive(repo *spec.Repo, id string, now time.Time) (string, error) {
	s, err := wholeSpec(repo, id)
	if err != nil {
		return "", err
	}
	if s.Archived {
		return "", fmt.Errorf("%s is already archived (%s).", s.ID, s.File)
	}
	if !s.Complete() {
		var why []string
		for _, st := range s.Steps {
			if !spec.StepMerged(st) {
				why = append(why, fmt.Sprintf("step %d isn't merged", st.N))
			}
		}
		human := false
		for _, c := range s.Criteria {
			if !c.Ticked {
				why = append(why, fmt.Sprintf("%s isn't ticked", c.ID))
				human = human || c.Proof == "human"
			}
		}
		if len(s.Steps) == 0 {
			why = append(why, "it has no steps")
		}
		msg := fmt.Sprintf("%s isn't finished: %s.", s.ID, strings.Join(why, ", "))
		if human {
			msg += " Run `furrow review` for what waits on a person."
		}
		return "", errors.New(msg)
	}
	text, err := readFile(repo, s.File)
	if err != nil {
		return "", err
	}
	plan, err := planArchive(repo, s, text, now)
	if err != nil {
		return "", err
	}
	if err := plan.apply(repo); err != nil {
		return "", err
	}
	return plan.message(), nil
}

// archivePlan is every change archive makes, worked out before any is written.
type archivePlan struct {
	id, from, to, area string
	specText, areaText string
}

func planArchive(repo *spec.Repo, s *spec.Spec, text string, now time.Time) (*archivePlan, error) {
	a := repo.Areas[s.Area]
	if a == nil {
		return nil, fmt.Errorf("%s's area %q has no file. Run `furrow lint`.", s.ID, s.Area)
	}
	specText, err := setStatus(text, spec.Done)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", s.File, err)
	}
	areaText, err := readFile(repo, a.File)
	if err != nil {
		return nil, err
	}
	areaText = appendToSection(areaText, "History", fmt.Sprintf("- %s: %s (archived %s)", s.ID, s.Title, now.Format(dateLayout)))
	to := filepath.ToSlash(filepath.Join(repo.Config.Specs, "archive", filepath.Base(s.File)))
	if _, err := os.Stat(filepath.Join(repo.Root, filepath.FromSlash(to))); err == nil {
		return nil, fmt.Errorf("%s already exists.", to)
	}
	return &archivePlan{id: s.ID, from: s.File, to: to, area: a.File, specText: specText, areaText: areaText}, nil
}

func (p *archivePlan) apply(repo *spec.Repo) error {
	from := filepath.Join(repo.Root, filepath.FromSlash(p.from))
	to := filepath.Join(repo.Root, filepath.FromSlash(p.to))
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil {
		return err
	}
	if err := writeFile(repo, p.to, p.specText); err != nil {
		return err
	}
	return writeFile(repo, p.area, p.areaText)
}

func (p *archivePlan) message() string {
	return fmt.Sprintf("Archived %s to %s and added it to %s's ## History.\n"+
		"Check that the area's ## Behaviour already says what %s changed.", p.id, p.to, p.area, p.id)
}
