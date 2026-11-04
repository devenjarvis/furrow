package importer

import (
	"fmt"
	"io"
	"strings"

	"github.com/devenjarvis/furrow/internal/spec"
)

// Text prints the plan for a person. With write false it ends by saying it was a dry run.
func (p *Plan) Text(w io.Writer, write bool) {
	counts := map[string]map[string]int{"archive": {}, "changes": {}}
	idW, statusW, areaW := 0, 0, 0
	for _, s := range p.Specs {
		dir := "changes"
		if strings.HasPrefix(s.To, p.specs+"/archive/") {
			dir = "archive"
		}
		counts[dir][s.Status]++
		idW, statusW, areaW = max(idW, len(s.ID)), max(statusW, len(s.Status)), max(areaW, len(s.Area))
	}
	group := func(dir string, order ...string) string {
		total := 0
		var parts []string
		for _, st := range order {
			if n := counts[dir][st]; n > 0 {
				total += n
				parts = append(parts, fmt.Sprintf("%d %s", n, st))
			}
		}
		out := fmt.Sprintf("%d to %s/%s/", total, p.specs, dir)
		if len(parts) > 0 {
			out += " (" + strings.Join(parts, ", ") + ")"
		}
		return out
	}
	fmt.Fprintf(w, "%d %s specs: %s, %s.\n\n", len(p.Specs), p.source,
		group("archive", spec.Done), group("changes", spec.Accepted, spec.Draft, spec.Paused))

	for _, s := range p.Specs {
		var notes []string
		if s.Merged {
			notes = append(notes, "merged")
		}
		if len(s.Signoffs) > 0 {
			notes = append(notes, "signs off "+strings.Join(s.Signoffs, ", "))
		}
		line := fmt.Sprintf("%-*s  %-*s  %-*s  %s -> %s", idW, s.ID, statusW, s.Status, areaW, s.Area, s.From, s.To)
		if len(notes) > 0 {
			line += "  " + strings.Join(notes, ", ")
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}

	if len(p.Areas) > 0 {
		fmt.Fprintln(w, "\nAreas:")
		for _, a := range p.Areas {
			line := fmt.Sprintf("  %-6s  %s", a.Action, a.File)
			if n := len(a.History); n == 1 {
				line += "  +1 History line"
			} else if n > 1 {
				line += fmt.Sprintf("  +%d History lines", n)
			}
			fmt.Fprintln(w, line)
		}
	}
	fmt.Fprintf(w, "\nAlso writes %s/README.md and %s/_template.md (Furrow's, replacing %s's), and a .gitkeep in changes/, archive/ and areas/.\n", p.specs, p.specs, p.source)
	if len(p.removes) > len(p.Specs) {
		var others []string
		for _, r := range p.removes {
			if p.spec(r) == nil && !p.from(r) {
				others = append(others, r)
			}
		}
		if len(others) > 0 {
			fmt.Fprintf(w, "Removes %s.\n", strings.Join(others, " and "))
		}
	}

	if len(p.Problems) == 0 {
		fmt.Fprintln(w, "\nNo problems.")
	} else {
		fmt.Fprintf(w, "\nProblems (%d):\n", len(p.Problems))
		for _, pr := range p.Problems {
			fmt.Fprintf(w, "  - %s\n", pr)
		}
	}
	if write {
		return
	}
	if len(p.Problems) == 0 {
		fmt.Fprintln(w, "\nDry run: nothing written. Run it again with --write to write the plan.")
	} else {
		fmt.Fprintf(w, "\nDry run: nothing written. Fix the problems in the source specs, in %s's format, then run it again.\n", p.source)
	}
}

// Summary is the line printed after a successful --write.
func (p *Plan) Summary() string {
	written := 0
	for _, a := range p.Areas {
		if a.Action != "keep" {
			written++
		}
	}
	return fmt.Sprintf("Imported %d specs and wrote %d area files. Run `furrow lint`, then fill in each area's ## Behaviour.", len(p.Specs), written)
}

// from reports whether rel is a planned spec's source file.
func (p *Plan) from(rel string) bool {
	for _, s := range p.Specs {
		if s.From == rel {
			return true
		}
	}
	return false
}
