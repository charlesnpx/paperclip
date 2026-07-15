package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charlesnpx/paperclip/internal/version"
)

type Result struct {
	Schema    int                     `json:"schema"`
	Name      string                  `json:"name"`
	Version   string                  `json:"version"`
	Operation string                  `json:"operation"`
	Kind      string                  `json:"kind"`
	Targets   map[string]TargetResult `json:"targets"`
	Warnings  []string                `json:"warnings"`
	Notices   []string                `json:"notices"`
}

type TargetResult struct {
	Files []FileResult `json:"files"`
}

type FileResult struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

type Options struct {
	Operation   string
	Target      string
	JSON        bool
	InstallRoot string
	Executable  string
}

func Run(args []string, stdout io.Writer, stderr io.Writer, executable string) int {
	opts, err := parse(args, executable)
	if err != nil {
		fmt.Fprintln(stderr, "papercut install:", err)
		return 2
	}
	result, err := Execute(opts)
	if err != nil {
		fmt.Fprintln(stderr, "papercut install:", err)
		return 1
	}
	if opts.JSON {
		body, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintln(stderr, "papercut install:", err)
			return 1
		}
		fmt.Fprintln(stdout, string(body))
		return 0
	}
	fmt.Fprintf(stdout, "%s %s %s\n", result.Name, result.Version, result.Operation)
	for _, notice := range result.Notices {
		if strings.TrimSpace(notice) == "" {
			continue
		}
		fmt.Fprintf(stdout, "notice: %s\n", notice)
	}
	return 0
}

func Execute(opts Options) (Result, error) {
	if opts.Operation == "" {
		opts.Operation = "install"
	}
	if opts.Target == "" {
		opts.Target = "all"
	}
	if opts.Executable == "" {
		return Result{}, errors.New("executable path is required")
	}
	if opts.InstallRoot != "" && !filepath.IsAbs(opts.InstallRoot) {
		return Result{}, errors.New("--install-root must be absolute")
	}
	result := Result{
		Schema:    1,
		Name:      "paperclip",
		Version:   version.String(),
		Operation: opts.Operation,
		Kind:      "delegated",
		Targets:   map[string]TargetResult{},
		Warnings:  []string{},
		Notices:   []string{},
	}
	targets, err := expandTargets(opts.Target)
	if err != nil {
		return Result{}, err
	}
	for _, target := range targets {
		result.Targets[target] = TargetResult{Files: []FileResult{}}
	}
	if contains(targets, "claude") {
		files, err := targetFiles(opts, "claude")
		if err != nil {
			return Result{}, err
		}
		targetResult, err := applyTargetFiles(opts.Operation, files)
		if err != nil {
			return Result{}, err
		}
		result.Targets["claude"] = targetResult
	}
	if contains(targets, "codex") {
		files, err := targetFiles(opts, "codex")
		if err != nil {
			return Result{}, err
		}
		targetResult, err := applyTargetFiles(opts.Operation, files)
		if err != nil {
			return Result{}, err
		}
		result.Targets["codex"] = targetResult
		if opts.Operation != "uninstall" {
			result.Notices = append(result.Notices, codexNotice(files[0].path))
		}
	}
	if contains(targets, "tools") {
		path, err := toolPath(opts.InstallRoot)
		if err != nil {
			return Result{}, err
		}
		file := FileResult{Path: path}
		switch opts.Operation {
		case "plan":
		case "install":
			sum, err := installTool(opts.Executable, path)
			if err != nil {
				return Result{}, err
			}
			file.SHA256 = sum
		case "uninstall":
			if err := uninstallTool(path); err != nil {
				return Result{}, err
			}
		default:
			return Result{}, errors.New("operation must be plan, install, or uninstall")
		}
		result.Targets["tools"] = TargetResult{Files: []FileResult{file}}
	}
	return result, nil
}

type installFile struct {
	path string
	body []byte
	mode os.FileMode
}

func targetFiles(opts Options, target string) ([]installFile, error) {
	root := opts.InstallRoot
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = home
	}
	switch target {
	case "claude":
		return []installFile{
			{
				path: filepath.Join(root, ".claude", "skills", "paperclip", "SKILL.md"),
				body: []byte(paperclipSkill),
				mode: 0o644,
			},
			{
				path: filepath.Join(root, ".claude", "rules", "paperclip.md"),
				body: []byte(claudeRule),
				mode: 0o644,
			},
		}, nil
	case "codex":
		return []installFile{
			{
				path: filepath.Join(root, ".codex", "skills", "paperclip", "SKILL.md"),
				body: []byte(codexSkill),
				mode: 0o644,
			},
		}, nil
	default:
		return nil, errors.New("unknown target")
	}
}

func applyTargetFiles(operation string, files []installFile) (TargetResult, error) {
	result := TargetResult{Files: make([]FileResult, 0, len(files))}
	for _, file := range files {
		item := FileResult{Path: file.path}
		switch operation {
		case "plan":
		case "install":
			sum, err := installDataFile(file)
			if err != nil {
				return TargetResult{}, err
			}
			item.SHA256 = sum
		case "uninstall":
			if err := uninstallTool(file.path); err != nil {
				return TargetResult{}, err
			}
		default:
			return TargetResult{}, errors.New("operation must be plan, install, or uninstall")
		}
		result.Files = append(result.Files, item)
	}
	return result, nil
}

func installDataFile(file installFile) (string, error) {
	if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
		return "", err
	}
	if err := writeFileAtomic(file.path, file.body, file.mode); err != nil {
		return "", err
	}
	sum := sha256.Sum256(file.body)
	return hex.EncodeToString(sum[:]), nil
}

func codexNotice(skillPath string) string {
	agentsPath := os.Getenv("CODEX_HOME")
	if agentsPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			agentsPath = filepath.Join(home, ".codex")
		} else {
			agentsPath = "$CODEX_HOME"
		}
	}
	agentsPath = filepath.Join(agentsPath, "AGENTS.md")
	return "Paperclip Codex skill target: " + skillPath + ". To enable routine use, add this text to your " + agentsPath + " file:\n\nUse the paperclip skill when operational friction, repo issues, local machine issues, harness/tool/model failures, broken links, or repeated process blockers should be recorded for later review. Do not paste secrets; paperclip stores local notes as provided."
}

func parse(args []string, executable string) (Options, error) {
	fs := flag.NewFlagSet("delegate-install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts Options
	operationFlags := 0
	setOperation := func(operation string) func(string) error {
		return func(value string) error {
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			if !enabled {
				return nil
			}
			operationFlags++
			if operationFlags > 1 {
				return errors.New("operation flags are mutually exclusive")
			}
			opts.Operation = operation
			return nil
		}
	}
	fs.BoolFunc("plan", "plan install", setOperation("plan"))
	fs.BoolFunc("install", "install", setOperation("install"))
	fs.BoolFunc("uninstall", "uninstall", setOperation("uninstall"))
	fs.StringVar(&opts.Target, "target", "all", "target")
	fs.BoolVar(&opts.JSON, "json", false, "json")
	fs.StringVar(&opts.InstallRoot, "install-root", "", "install root")
	opts.Executable = executable
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if fs.NArg() != 0 {
		return Options{}, errors.New("unexpected positional arguments")
	}
	return opts, nil
}

func expandTargets(target string) ([]string, error) {
	switch target {
	case "all":
		return []string{"claude", "codex", "tools"}, nil
	case "claude", "codex", "tools":
		return []string{target}, nil
	default:
		return nil, errors.New("--target must be claude, codex, tools, or all")
	}
}

func toolPath(root string) (string, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = home
	}
	return filepath.Join(root, ".local", "bin", "papercut"), nil
}

func installTool(src string, dst string) (string, error) {
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("executable path is a directory")
	}
	body, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := writeFileAtomic(dst, body, 0o755); err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func writeFileAtomic(dst string, body []byte, mode os.FileMode) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, "paperclip-install-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if strings.HasPrefix(filepath.Base(tmpPath), ".") {
		return errors.New("refusing hidden temporary file")
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return err
	}
	cleanup = false
	return syncDir(dir)
}

func uninstallTool(dst string) error {
	err := os.Remove(dst)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func syncDir(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func SortedTargets(result Result) []string {
	targets := make([]string, 0, len(result.Targets))
	for target := range result.Targets {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets
}

const codexSkill = `---
name: paperclip
description: Explicit-use Paperclip workflows for recording and reviewing local operational friction. Use only when the user explicitly invokes paperclip or an installed global/project instruction tells you to use Paperclip.
metadata:
  policy:
    allow_implicit_invocation: false
---

# Paperclip

Use the local ` + "`papercut`" + ` CLI to record and review operational friction encountered while working: repo issues, machine configuration problems, harness or tool failures, model failures, broken links, and process blockers.

Paperclip writes to the local personal ledger at ` + "`${PAPERCLIP_HOME:-~/paperclip}/PAPERCLIP.md`" + `. It stores the text provided by the user or agent; do not paste secrets.

## Workflows

- Add a papercut: run ` + "`papercut add --expected ... --observed ... --impact ... --locus repo`" + `. Include ` + "`--severity`" + `, ` + "`--scope`" + `, and ` + "`--suggestion`" + ` when useful.
- Add sensitive details: prefer ` + "`papercut add --input-json -`" + ` so values do not enter shell history. Still do not paste credentials, tokens, private keys, or other secrets.
- Review current repo: run ` + "`papercut review`" + ` from the repository being worked on.
- List everything: run ` + "`papercut list --repo all`" + `.
- Dispose noise: run ` + "`papercut dispose <observation-id> --reason ...`" + ` when an item is obsolete, duplicate, or not actionable.

Use stdout from ` + "`papercut`" + ` as the source of truth for created IDs and lifecycle results. Do not edit ` + "`PAPERCLIP.md`" + ` event blocks by hand.
`

const paperclipSkill = `---
name: paperclip
description: Explicit-use Paperclip workflows for recording and reviewing local operational friction. Use only when the user explicitly invokes paperclip or an installed global/project instruction tells you to use Paperclip.
---

# Paperclip

Use the local ` + "`papercut`" + ` CLI to record and review operational friction encountered while working: repo issues, machine configuration problems, harness or tool failures, model failures, broken links, and process blockers.

Paperclip writes to the local personal ledger at ` + "`${PAPERCLIP_HOME:-~/paperclip}/PAPERCLIP.md`" + `. It stores the text provided by the user or agent; do not paste secrets.

## Workflows

- Add a papercut: run ` + "`papercut add --expected ... --observed ... --impact ... --locus repo`" + `. Include ` + "`--severity`" + `, ` + "`--scope`" + `, and ` + "`--suggestion`" + ` when useful.
- Add sensitive details: prefer ` + "`papercut add --input-json -`" + ` so values do not enter shell history. Still do not paste credentials, tokens, private keys, or other secrets.
- Review current repo: run ` + "`papercut review`" + ` from the repository being worked on.
- List everything: run ` + "`papercut list --repo all`" + `.
- Dispose noise: run ` + "`papercut dispose <observation-id> --reason ...`" + ` when an item is obsolete, duplicate, or not actionable.

Use stdout from ` + "`papercut`" + ` as the source of truth for created IDs and lifecycle results. Do not edit ` + "`PAPERCLIP.md`" + ` event blocks by hand.
`

const claudeRule = `Use the Paperclip skill when operational friction, repo issues, local machine issues, harness/tool/model failures, broken links, or repeated process blockers should be recorded for later review. Do not paste secrets; Paperclip stores local notes as provided.
`
