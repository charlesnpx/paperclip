package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charlesnpx/paperclip/internal/app"
	"github.com/charlesnpx/paperclip/internal/domain"
	"github.com/charlesnpx/paperclip/internal/install"
	"github.com/charlesnpx/paperclip/internal/review"
)

const (
	ExitSuccess     = 0
	ExitUnexpected  = 1
	ExitUsage       = 2
	ExitPolicy      = 3
	ExitConflict    = 4
	ExitMalformed   = 5
	ExitLockTimeout = 6
)

type Application interface {
	Add(domain.RawRequest) (domain.CommitResult, error)
	List(app.QueryOptions) ([]domain.Observation, error)
	ClaimFixed(string) (domain.CommitResult, error)
	VerifyFixed(string) (domain.CommitResult, error)
	Dispose(string, string) (domain.CommitResult, error)
}

func Run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, application Application) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "papercut: command is required")
		return ExitUsage
	}
	switch args[0] {
	case "add":
		return runAdd(args[1:], stdin, stdout, stderr, application)
	case "list":
		return runList(args[1:], stdout, stderr, application, false)
	case "review":
		return runList(args[1:], stdout, stderr, application, true)
	case "claim-fixed":
		return runTransition(args[1:], stdout, stderr, application.ClaimFixed)
	case "verify-fixed":
		return runTransition(args[1:], stdout, stderr, application.VerifyFixed)
	case "dispose":
		return runDispose(args[1:], stdout, stderr, application)
	case "instructions":
		return runInstructions(args[1:], stdout, stderr)
	case "delegate-install":
		return RunInstaller(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, "papercut: unknown command")
		return ExitUsage
	}
}

func RunInstaller(args []string, stdout io.Writer, stderr io.Writer) int {
	exe, _ := os.Executable()
	return install.Run(args, stdout, stderr, exe)
}

func runAdd(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, application Application) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var raw domain.RawRequest
	var inputJSON string
	fs.StringVar(&raw.Expected, "expected", "", "expected")
	fs.StringVar(&raw.Observed, "observed", "", "observed")
	fs.StringVar(&raw.Impact, "impact", "", "impact")
	fs.StringVar(&raw.Locus, "locus", "", "locus")
	fs.StringVar(&raw.Severity, "severity", "", "severity")
	fs.StringVar(&raw.Scope, "scope", "", "scope")
	fs.StringVar(&raw.Suggestion, "suggestion", "", "suggestion")
	fs.StringVar(&raw.IdempotencyKey, "idempotency-key", "", "idempotency key")
	fs.StringVar(&raw.IdempotencyKey, "idempotency_key", "", "idempotency key")
	fs.StringVar(&inputJSON, "input-json", "", "input json")
	if err := fs.Parse(args); err != nil {
		return diagnostic(stderr, usage(err))
	}
	if fs.NArg() != 0 {
		return diagnostic(stderr, usage(errors.New("add takes no positional arguments")))
	}
	if inputJSON != "" {
		if captureFlagsSet(fs) {
			return diagnostic(stderr, usage(errors.New("add rejects mixed --input-json and flag input modes")))
		}
		if inputJSON != "-" {
			return diagnostic(stderr, usage(errors.New("--input-json only accepts -")))
		}
		dec := json.NewDecoder(stdin)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&raw); err != nil {
			return diagnostic(stderr, fmt.Errorf("%w: invalid input json", app.ErrUsage))
		}
		if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			if err == nil {
				return diagnostic(stderr, fmt.Errorf("%w: invalid input json: trailing content", app.ErrUsage))
			}
			return diagnostic(stderr, fmt.Errorf("%w: invalid input json", app.ErrUsage))
		}
	}
	result, err := application.Add(raw)
	if err != nil {
		return diagnostic(stderr, err)
	}
	out, err := review.RenderJSON(result)
	if err != nil {
		return diagnostic(stderr, err)
	}
	fmt.Fprint(stdout, out)
	return ExitSuccess
}

func captureFlagsSet(fs *flag.FlagSet) bool {
	captureNames := map[string]struct{}{
		"expected": {}, "observed": {}, "impact": {}, "locus": {}, "severity": {},
		"scope": {}, "suggestion": {}, "idempotency-key": {}, "idempotency_key": {},
	}
	set := false
	fs.Visit(func(flag *flag.Flag) {
		if _, ok := captureNames[flag.Name]; ok {
			set = true
		}
	})
	return set
}

func runList(args []string, stdout io.Writer, stderr io.Writer, application Application, grouped bool) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts app.QueryOptions
	var jsonOut bool
	fs.StringVar(&opts.Repo, "repo", "current", "repo")
	fs.StringVar(&opts.Locus, "locus", "", "locus")
	fs.StringVar(&opts.Scope, "scope", "", "scope")
	fs.BoolVar(&jsonOut, "json", false, "json")
	if err := fs.Parse(args); err != nil {
		return diagnostic(stderr, usage(err))
	}
	if fs.NArg() != 0 {
		return diagnostic(stderr, usage(errors.New("list/review takes no positional arguments")))
	}
	observations, err := application.List(opts)
	if err != nil {
		return diagnostic(stderr, err)
	}
	if grouped {
		groups := review.Groups(observations)
		if jsonOut {
			out, err := review.RenderJSON(groups)
			if err != nil {
				return diagnostic(stderr, err)
			}
			fmt.Fprint(stdout, out)
			return ExitSuccess
		}
		fmt.Fprint(stdout, review.RenderReview(groups))
		return ExitSuccess
	}
	if jsonOut {
		out, err := review.RenderJSON(observations)
		if err != nil {
			return diagnostic(stderr, err)
		}
		fmt.Fprint(stdout, out)
		return ExitSuccess
	}
	fmt.Fprint(stdout, review.RenderList(observations))
	return ExitSuccess
}

func runTransition(args []string, stdout io.Writer, stderr io.Writer, fn func(string) (domain.CommitResult, error)) int {
	if len(args) != 1 {
		return diagnostic(stderr, usage(errors.New("transition command takes exactly one observation id")))
	}
	if strings.HasPrefix(args[0], "-") {
		return diagnostic(stderr, usage(errors.New("transition command takes exactly one observation id")))
	}
	result, err := fn(args[0])
	if err != nil {
		return diagnostic(stderr, err)
	}
	out, err := review.RenderJSON(result)
	if err != nil {
		return diagnostic(stderr, err)
	}
	fmt.Fprint(stdout, out)
	return ExitSuccess
}

func runDispose(args []string, stdout io.Writer, stderr io.Writer, application Application) int {
	id, reason, err := parseDisposeArgs(args)
	if err != nil {
		return diagnostic(stderr, usage(err))
	}
	result, err := application.Dispose(id, reason)
	if err != nil {
		return diagnostic(stderr, err)
	}
	out, err := review.RenderJSON(result)
	if err != nil {
		return diagnostic(stderr, err)
	}
	fmt.Fprint(stdout, out)
	return ExitSuccess
}

func parseDisposeArgs(args []string) (string, string, error) {
	var id string
	var reason string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--reason":
			if i+1 >= len(args) {
				return "", "", errors.New("dispose requires --reason value")
			}
			i++
			reason = args[i]
		case strings.HasPrefix(arg, "--reason="):
			reason = strings.TrimPrefix(arg, "--reason=")
		case strings.HasPrefix(arg, "-"):
			return "", "", fmt.Errorf("unknown dispose flag %s", arg)
		default:
			if id != "" {
				return "", "", errors.New("dispose takes exactly one observation id")
			}
			id = arg
		}
	}
	if id == "" {
		return "", "", errors.New("dispose takes exactly one observation id")
	}
	if reason == "" {
		return "", "", errors.New("dispose requires --reason")
	}
	return id, reason, nil
}

func runInstructions(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("instructions", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var format string
	fs.StringVar(&format, "format", "", "format")
	if err := fs.Parse(args); err != nil {
		return diagnostic(stderr, usage(err))
	}
	if fs.NArg() != 0 {
		return diagnostic(stderr, usage(errors.New("instructions takes no positional arguments")))
	}
	if format != "agents" {
		return diagnostic(stderr, usage(errors.New("instructions requires --format agents")))
	}
	fmt.Fprint(stdout, review.InstructionsAgents())
	return ExitSuccess
}

func diagnostic(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "papercut:", err)
	return app.ExitCode(err)
}

func usage(err error) error {
	return fmt.Errorf("%w: %v", app.ErrUsage, err)
}
