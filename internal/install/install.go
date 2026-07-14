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
	}
	targets, err := expandTargets(opts.Target)
	if err != nil {
		return Result{}, err
	}
	for _, target := range targets {
		result.Targets[target] = TargetResult{Files: []FileResult{}}
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
	tmp, err := os.CreateTemp(dir, "papercut-install-*.tmp")
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
