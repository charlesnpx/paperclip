package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charlesnpx/paperclip/internal/app"
	"github.com/charlesnpx/paperclip/internal/domain"
	"github.com/charlesnpx/paperclip/internal/ledger"
	"github.com/charlesnpx/paperclip/internal/policy"
)

type cliContext struct{}

func (cliContext) Current() (domain.Context, error) {
	return domain.Context{RepoID: "none"}, nil
}

func TestCLIAddListAndReviewJSON(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	ids := []string{"obs_1", "evt_1", "evt_2"}
	next := 0
	application := app.New(repo, cliContext{}, policy.DefaultScanner()).
		WithClock(func() time.Time { return time.Date(2026, 7, 14, 12, 0, next, 0, time.UTC) }).
		WithIDGenerator(func(prefix string) (string, error) {
			id := ids[next]
			next++
			return id, nil
		})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"add", "--expected", "tests pass", "--observed", "tests fail", "--impact", "blocked", "--locus", "repo", "--suggestion", "fix harness"}, strings.NewReader(""), &stdout, &stderr, application)
	if code != ExitSuccess {
		t.Fatalf("add code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"observation_id": "obs_1"`) {
		t.Fatalf("add stdout=%s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"list", "--repo", "all", "--json"}, strings.NewReader(""), &stdout, &stderr, application)
	if code != ExitSuccess {
		t.Fatalf("list code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id": "obs_1"`) {
		t.Fatalf("list stdout=%s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"review", "--repo", "all", "--json"}, strings.NewReader(""), &stdout, &stderr, application)
	if code != ExitSuccess {
		t.Fatalf("review code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"observations"`) {
		t.Fatalf("review stdout=%s", stdout.String())
	}
}

func TestCLIExitCodesAndDiagnostics(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	application := app.New(repo, cliContext{}, policy.DefaultScanner())
	var stdout, stderr bytes.Buffer
	code := Run([]string{"add", "--expected", "e"}, strings.NewReader(""), &stdout, &stderr, application)
	if code != ExitUsage {
		t.Fatalf("usage code=%d stderr=%s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	secret := "token=ghp_abcdefghijklmnopqrstuvwxyz123456"
	code = Run([]string{"add", "--expected", "e", "--observed", secret, "--impact", "i", "--locus", "repo"}, strings.NewReader(""), &stdout, &stderr, application)
	if code != ExitPolicy {
		t.Fatalf("policy code=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "ghp_") || strings.Contains(stderr.String(), "token=") {
		t.Fatalf("diagnostic leaked secret: %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"list", "--locus", "typo"}, strings.NewReader(""), &stdout, &stderr, application)
	if code != ExitUsage {
		t.Fatalf("invalid filter code=%d stderr=%s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"list", "--repo", "curent"}, strings.NewReader(""), &stdout, &stderr, application)
	if code != ExitUsage {
		t.Fatalf("invalid repo filter code=%d stderr=%s", code, stderr.String())
	}
	for _, args := range [][]string{
		{"claim-fixed"},
		{"claim-fixed", "--bogus"},
		{"dispose", "obs_1"},
		{"list", "--bogus"},
		{"add", "stray"},
	} {
		stdout.Reset()
		stderr.Reset()
		code = Run(args, strings.NewReader(""), &stdout, &stderr, application)
		if code != ExitUsage {
			t.Fatalf("%v code=%d stderr=%s", args, code, stderr.String())
		}
	}
}

func TestCLIDisposeSupportsDocumentedArgumentOrder(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	ids := []string{"obs_1", "evt_open", "evt_dispose"}
	next := 0
	application := app.New(repo, cliContext{}, policy.DefaultScanner()).
		WithClock(func() time.Time { return time.Date(2026, 7, 14, 12, 0, next, 0, time.UTC) }).
		WithIDGenerator(func(prefix string) (string, error) {
			id := ids[next]
			next++
			return id, nil
		})
	var stdout, stderr bytes.Buffer
	code := Run([]string{"add", "--expected", "e", "--observed", "o", "--impact", "i", "--locus", "repo"}, strings.NewReader(""), &stdout, &stderr, application)
	if code != ExitSuccess {
		t.Fatalf("add code=%d stderr=%s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"dispose", "obs_1", "--reason", "obsolete"}, strings.NewReader(""), &stdout, &stderr, application)
	if code != ExitSuccess {
		t.Fatalf("dispose code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"observation_id": "obs_1"`) {
		t.Fatalf("dispose stdout=%s", stdout.String())
	}
}

func TestCLIInputJSONRejectsTrailingContent(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	application := app.New(repo, cliContext{}, policy.DefaultScanner())
	body := `{"expected":"e","observed":"o","impact":"i","locus":"repo"} {"expected":"ignored","observed":"ignored","impact":"ignored","locus":"repo"}`
	var stdout, stderr bytes.Buffer
	code := Run([]string{"add", "--input-json", "-"}, strings.NewReader(body), &stdout, &stderr, application)
	if code != ExitUsage {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}

func TestCLIInputJSONRejectsExplicitEmptyCaptureFlag(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	application := app.New(repo, cliContext{}, policy.DefaultScanner())
	body := `{"expected":"e","observed":"o","impact":"i","locus":"repo"}`
	var stdout, stderr bytes.Buffer
	code := Run([]string{"add", "--expected=", "--input-json", "-"}, strings.NewReader(body), &stdout, &stderr, application)
	if code != ExitUsage {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}

func TestCLIInputJSONDecodeErrorDoesNotEchoUnknownField(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	application := app.New(repo, cliContext{}, policy.DefaultScanner())
	body := `{"expected":"e","observed":"o","impact":"i","locus":"repo","authorization=Basic dXNlcjpwYXNz":"secret"}`
	var stdout, stderr bytes.Buffer
	code := Run([]string{"add", "--input-json", "-"}, strings.NewReader(body), &stdout, &stderr, application)
	if code != ExitUsage {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "authorization") || strings.Contains(stderr.String(), "dXNlcjpwYXNz") {
		t.Fatalf("diagnostic leaked unknown field: %s", stderr.String())
	}
}
