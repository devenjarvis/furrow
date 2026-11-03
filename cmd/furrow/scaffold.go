package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/devenjarvis/furrow/internal/config"
	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/scaffold"
)

func init() {
	register("init", "Start Furrow in this repo, keeping any files that exist", runInit)
	register("new", "Start a draft spec with the next number", runNew)
	register("area", "Start a living spec for one part of the product", runArea)
	register("invariants", "List the product's invariants", runInvariants)
	register("doctor", "Say whether bin/furrow's pinned version is behind the latest release", runDoctor)
}

func runInit(env *Env, args []string) int {
	fs := newFlags(env, "init", "[--prefix P] [--pin --checksums FILE [--version V]]")
	prefix := fs.String("prefix", "", "spec id prefix (default: the folder's name, upper-cased)")
	pin := fs.Bool("pin", false, "also write bin/furrow, a shim that downloads and checks a pinned release")
	checksums := fs.String("checksums", "", "with --pin: the release's checksums.txt")
	pinVersion := fs.String("version", version, "with --pin: the release to pin")
	if pos, err := parseFlags(fs, args); err != nil {
		return 2
	} else if len(pos) > 0 {
		fs.Usage()
		return 2
	}
	// Check the pin before init writes anything, so a bad one leaves the repo as it was.
	var shim string
	if *pin {
		if *checksums == "" {
			v := *pinVersion
			if v == "dev" {
				v = "v0.1.0"
			}
			return fail(env, "Pinning needs the release's checksums. Download %s and pass --checksums checksums.txt.", scaffold.ChecksumsURL(v))
		}
		data, err := os.ReadFile(resolve(env.Dir, *checksums))
		if err != nil {
			return fail(env, "Couldn't read the checksums: %v", err)
		}
		if shim, err = scaffold.ShimText(*pinVersion, data); err != nil {
			return fail(env, "%v", err)
		}
	} else if *checksums != "" {
		return fail(env, "--checksums only works with --pin.")
	}
	res, err := scaffold.Init(env.Dir, *prefix)
	if err == nil && *pin {
		var pinned scaffold.InitResult
		pinned, err = scaffold.WritePin(env.Dir, shim)
		res.Entries = append(res.Entries, pinned.Entries...)
		res.Notes = append(res.Notes, pinned.Notes...)
	}
	res.Text(env.Stdout)
	if err != nil {
		return fail(env, "%v", err)
	}
	return 0
}

// resolve makes a path from the command line absolute against the command's folder.
func resolve(dir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}

func runDoctor(env *Env, args []string) int {
	fs := newFlags(env, "doctor", "[--repo URL] [--json]")
	repo := fs.String("repo", scaffold.ReleaseRepo, "the git repo whose v* tags are Furrow's releases")
	asJSON := fs.Bool("json", false, "print JSON")
	if pos, err := parseFlags(fs, args); err != nil {
		return 2
	} else if len(pos) > 0 {
		fs.Usage()
		return 2
	}
	root, _, err := config.Find(env.Dir)
	if err != nil {
		return fail(env, "%v", err)
	}
	st, err := scaffold.Doctor(root, gitx.Git{Dir: root}, *repo)
	if err != nil {
		return fail(env, "%v", err)
	}
	if *asJSON {
		return printJSON(env, st)
	}
	st.Text(env.Stdout)
	return 0
}

func runNew(env *Env, args []string) int {
	fs := newFlags(env, "new", "<slug> --title T --area A [--json]")
	title := fs.String("title", "", "the spec's title")
	area := fs.String("area", "", "the area it changes (specs/areas/<area>.md)")
	asJSON := fs.Bool("json", false, "print JSON")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fs.Usage()
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	c, err := scaffold.New(repo, pos[0], *title, *area)
	if err != nil {
		return fail(env, "%v", err)
	}
	if *asJSON {
		return printJSON(env, c)
	}
	fmt.Fprintln(env.Stdout, c.File)
	return 0
}

func runArea(env *Env, args []string) int {
	fs := newFlags(env, "area", "<name> --title T")
	title := fs.String("title", "", "the area's title")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fs.Usage()
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	file, err := scaffold.Area(repo, pos[0], *title)
	if err != nil {
		return fail(env, "%v", err)
	}
	fmt.Fprintln(env.Stdout, file)
	return 0
}

func runInvariants(env *Env, args []string) int {
	fs := newFlags(env, "invariants", "[--json]")
	asJSON := fs.Bool("json", false, "print JSON")
	if pos, err := parseFlags(fs, args); err != nil {
		return 2
	} else if len(pos) > 0 {
		fs.Usage()
		return 2
	}
	root, cfg, err := config.Find(env.Dir)
	if err != nil {
		return fail(env, "%v", err)
	}
	invs, err := scaffold.Invariants(filepath.Join(root, cfg.Product))
	if err != nil {
		return fail(env, "%v", err)
	}
	if *asJSON {
		return printJSON(env, invs)
	}
	if len(invs) == 0 {
		fmt.Fprintf(env.Stdout, "No invariants in %s. Add an \"## Invariants\" section with lines like **INV-1: <rule>.**\n", cfg.Product)
		return 0
	}
	scaffold.WriteInvariants(env.Stdout, invs)
	return 0
}
