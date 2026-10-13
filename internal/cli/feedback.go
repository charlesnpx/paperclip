package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/charlesnpx/paperclip/internal/domain"
	"github.com/charlesnpx/paperclip/internal/review"
)

type feedbackRequest struct {
	domain.Feedback
	domain.Conversation
	Suggestion     string `json:"suggestion"`
	Severity       string `json:"severity"`
	IdempotencyKey string `json:"idempotency_key"`
}

const FeedbackInstruction = "After using a skill, use the skill-feedback skill to record useful observations about ergonomics, instructions, APIs, bugs, improvements, or workarounds, subject to its include/exclude configuration. Include the skill name and a concrete note; attach available conversation references. Record feedback even when the task succeeds, but do not invent issues or require a report for every use. Keep feedback local, omit secrets, avoid duplicating it in the general Paperclip ledger, and continue the user's task."

func RunFeedback(args []string, stdin io.Reader, stdout, stderr io.Writer, application Application) int {
	global := flag.NewFlagSet("feedback", flag.ContinueOnError)
	global.SetOutput(io.Discard)
	configPath := global.String("config", "", "configuration path")
	if err := global.Parse(args); err != nil && !errors.Is(err, flag.ErrHelp) {
		return diagnostic(stderr, usage(err))
	}
	args = global.Args()
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(stdout, `paperclip feedback [--config PATH] COMMAND
  add --skill NAME --note TEXT
    Optional: --category TEXT --workaround TEXT --suggestion TEXT --source TEXT
              --version TEXT --severity LEVEL --idempotency-key KEY
              --harness NAME --session-id ID --session-path PATH --session-url URL
    JSON input: add --input-json -
  check --skill NAME
  config validate
  list|review [--skill NAME] [--repo all|current|none|REPO_ID] [--json]
  claim-fixed|verify-fixed ID
  dispose ID --reason TEXT
  instructions

Uses ${PAPERCLIP_FEEDBACK_HOME:-~/paperclip/skill-feedback}/PAPERCLIP.md.
Configuration defaults to config.json in that directory.
List and review include all repositories by default.`)
		return ExitSuccess
	}
	switch args[0] {
	case "add":
		config, err := loadFeedbackConfig(*configPath)
		if err != nil {
			return diagnostic(stderr, usage(err))
		}
		return runFeedbackAdd(args[1:], stdin, stdout, stderr, application, config)
	case "config":
		if len(args) != 2 || args[1] != "validate" {
			return diagnostic(stderr, usage(errors.New("use feedback config validate")))
		}
		config, err := loadFeedbackConfig(*configPath)
		if err != nil {
			return diagnostic(stderr, usage(err))
		}
		return feedbackJSON(stdout, stderr, config)
	case "check":
		fs := flag.NewFlagSet("feedback check", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		skill := fs.String("skill", "", "exact skill name")
		if err := fs.Parse(args[1:]); err != nil {
			return diagnostic(stderr, usage(err))
		}
		if fs.NArg() != 0 {
			return diagnostic(stderr, usage(errors.New("check takes no positional arguments")))
		}
		if err := domain.ValidateSkillName(*skill); err != nil {
			return diagnostic(stderr, usage(err))
		}
		config, err := loadFeedbackConfig(*configPath)
		if err != nil {
			return diagnostic(stderr, usage(err))
		}
		reason := config.reason(*skill)
		return feedbackJSON(stdout, stderr, map[string]any{"skill": *skill, "eligible": reason == "", "reason": reason})
	case "list", "review":
		fs := flag.NewFlagSet("feedback "+args[0], flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		skill := fs.String("skill", "", "exact skill name")
		repo := fs.String("repo", "all", "repository filter")
		jsonOut := fs.Bool("json", false, "JSON output")
		if err := fs.Parse(args[1:]); err != nil {
			return diagnostic(stderr, usage(err))
		}
		if fs.NArg() != 0 {
			return diagnostic(stderr, usage(errors.New("feedback list/review takes no positional arguments")))
		}
		forward := []string{"--repo", *repo, "--scope", strings.TrimSpace(*skill)}
		if *jsonOut {
			forward = append(forward, "--json")
		}
		return runList(forward, stdout, stderr, application, args[0] == "review")
	case "claim-fixed":
		return runTransition(args[1:], stdout, stderr, application.ClaimFixed)
	case "verify-fixed":
		return runTransition(args[1:], stdout, stderr, application.VerifyFixed)
	case "dispose":
		return runDispose(args[1:], stdout, stderr, application)
	case "instructions":
		if len(args) != 1 {
			return diagnostic(stderr, usage(errors.New("feedback instructions takes no arguments")))
		}
		fmt.Fprintln(stdout, FeedbackInstruction)
		return ExitSuccess
	default:
		return diagnostic(stderr, usage(errors.New("unknown feedback command")))
	}
}

func feedbackJSON(stdout, stderr io.Writer, value any) int {
	body, err := review.RenderJSON(value)
	if err != nil {
		return diagnostic(stderr, err)
	}
	fmt.Fprint(stdout, body)
	return ExitSuccess
}

func runFeedbackAdd(args []string, stdin io.Reader, stdout, stderr io.Writer, application Application, config feedbackConfig) int {
	fs := flag.NewFlagSet("feedback add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var request feedbackRequest
	fs.StringVar(&request.Skill, "skill", "", "skill name")
	fs.StringVar(&request.Note, "note", "", "free-form feedback")
	fs.StringVar(&request.Category, "category", "", "feedback category")
	fs.StringVar(&request.Workaround, "workaround", "", "workaround actually used")
	fs.StringVar(&request.Suggestion, "suggestion", "", "proposed improvement")
	fs.StringVar(&request.Source, "source", "", "skill source path or URL")
	fs.StringVar(&request.Version, "version", "", "skill version or commit")
	fs.StringVar(&request.Severity, "severity", "", "severity")
	fs.StringVar(&request.IdempotencyKey, "idempotency-key", "", "idempotency key")
	conversationFlags(fs, &request.Conversation)
	inputJSON := fs.String("input-json", "", "read JSON from stdin with -")
	if err := fs.Parse(args); err != nil {
		return diagnostic(stderr, usage(err))
	}
	if fs.NArg() != 0 {
		return diagnostic(stderr, usage(errors.New("feedback add takes no positional arguments")))
	}
	jsonSet, captureSet := false, false
	fs.Visit(func(option *flag.Flag) {
		if option.Name == "input-json" {
			jsonSet = true
		} else {
			captureSet = true
		}
	})
	if jsonSet {
		if *inputJSON != "-" || captureSet {
			return diagnostic(stderr, usage(errors.New("use --input-json - without capture flags")))
		}
		decoder := json.NewDecoder(stdin)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			return diagnostic(stderr, usage(errors.New("invalid feedback JSON")))
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return diagnostic(stderr, usage(errors.New("trailing feedback JSON content")))
		}
	}
	request.Feedback = request.Feedback.Normalize()
	raw := domain.RawRequest{
		Feedback: &request.Feedback, Conversation: request.Conversation,
		Severity: request.Severity, Suggestion: request.Suggestion, IdempotencyKey: request.IdempotencyKey,
	}
	if _, err := domain.ValidateRawRequest(raw); err != nil {
		return diagnostic(stderr, usage(err))
	}
	if reason := config.reason(request.Skill); reason != "" {
		return feedbackJSON(stdout, stderr, map[string]any{"skill": request.Skill, "skipped": true, "reason": reason})
	}
	result, err := application.Add(raw)
	if err != nil {
		return diagnostic(stderr, err)
	}
	return feedbackJSON(stdout, stderr, result)
}
