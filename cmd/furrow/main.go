// Command furrow schedules spec steps across a swarm of coding agents. See docs/product.md.
//
// Each command lives in its own file and registers itself in init(), so a new command never
// touches this file.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/devenjarvis/furrow/internal/spec"
)

// version is set at build time with -ldflags "-X main.version=v0.1.0".
var version = "dev"

// Env is what a command can see of the world. Tests can build their own.
type Env struct {
	Dir    string
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string
	Now    func() time.Time
}

type command struct {
	name    string
	summary string
	run     func(env *Env, args []string) int
}

var commands = map[string]command{}

// register adds a command. Call it from init() in the command's own file.
func register(name, summary string, run func(env *Env, args []string) int) {
	if _, dup := commands[name]; dup {
		panic("furrow: command registered twice: " + name)
	}
	commands[name] = command{name: name, summary: summary, run: run}
}

func init() {
	register("version", "Print furrow's version", func(env *Env, _ []string) int {
		fmt.Fprintf(env.Stdout, "furrow %s\n", version)
		return 0
	})
}

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	env := &Env{Dir: dir, Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.Getenv, Now: time.Now}
	os.Exit(run(env, os.Args[1:]))
}

func run(env *Env, args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage(env.Stdout)
		return 0
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(env.Stderr, "Unknown command %q. Run `furrow help` for the list.\n", args[0])
		return 2
	}
	return cmd.run(env, args[1:])
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "furrow: specs in, parallel pull requests out.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage: furrow <command> [flags]")
	fmt.Fprintln(w)
	names := make([]string, 0, len(commands))
	width := 0
	for n := range commands {
		names = append(names, n)
		width = max(width, len(n))
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(w, "  %-*s  %s\n", width, n, commands[n].summary)
	}
}

// parseFlags parses flags that may come before or after positional arguments.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// newFlags makes a flag set that prints errors to the command's stderr.
func newFlags(env *Env, name, usageLine string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(env.Stderr, "Usage: furrow %s %s\n", name, usageLine)
		fs.PrintDefaults()
	}
	return fs
}

// loadRepo loads the repo around env.Dir and reports a failure on stderr.
func loadRepo(env *Env) (*spec.Repo, bool) {
	repo, err := spec.LoadDir(env.Dir)
	if err != nil {
		fmt.Fprintln(env.Stderr, err)
		return nil, false
	}
	return repo, true
}

// fail prints a message to stderr and returns exit code 1.
func fail(env *Env, format string, args ...any) int {
	msg := fmt.Sprintf(format, args...)
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Fprint(env.Stderr, msg)
	return 1
}
