package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charlesnpx/paperclip/internal/app"
	"github.com/charlesnpx/paperclip/internal/domain"
	"github.com/charlesnpx/paperclip/internal/ledger"
)

func TestFeedbackConfigValidationAndEligibility(t *testing.T) {
	for _, example := range []struct {
		name, body, skill string
		valid, eligible   bool
	}{
		{"missing lists", `{}`, "unregistered", true, true},
		{"empty lists", `{"includes":[],"excludes":[]}`, "new-skill", true, true},
		{"include", `{"includes":["relay:steer"]}`, "relay:steer", true, true},
		{"exact", `{"includes":["relay"]}`, "relay:steer", true, false},
		{"case sensitive", `{"includes":["Relay"]}`, "relay", true, false},
		{"exclusion wins", `{"includes":["relay"],"excludes":["relay"]}`, "relay", true, false},
		{"exclusion only", `{"excludes":["relay"]}`, "custom", true, true},
		{"unknown key", `{"include":[]}`, "relay", false, false},
		{"null document", `null`, "relay", false, false},
		{"null list", `{"includes":null}`, "relay", false, false},
		{"wrong type", `{"includes":"relay"}`, "relay", false, false},
		{"empty name", `{"includes":[""]}`, "relay", false, false},
		{"null name", `{"includes":[null]}`, "relay", false, false},
		{"whitespace", `{"includes":["two names"]}`, "relay", false, false},
		{"trailing JSON", `{} {}`, "relay", false, false},
	} {
		t.Run(example.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(example.body), 0o600); err != nil {
				t.Fatal(err)
			}
			config, err := loadFeedbackConfig(path)
			if (err == nil) != example.valid {
				t.Fatalf("valid=%v error=%v", example.valid, err)
			}
			if err == nil && (config.reason(example.skill) == "") != example.eligible {
				t.Fatalf("unexpected eligibility: %s", config.reason(example.skill))
			}
		})
	}
}

func TestFeedbackSkippedAndChecksDoNotTouchLedger(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PAPERCLIP_FEEDBACK_HOME", filepath.Join(root, "absent"))
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"excludes":["relay"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{
		{"--config", configPath, "add", "--skill", "relay", "--note", "a useful note"},
		{"--config", configPath, "check", "--skill", "relay"},
		{"--config", configPath, "config", "validate"},
		{"check", "--skill", "unregistered"},
		{"config", "validate"},
	} {
		var stdout, stderr bytes.Buffer
		if code := RunFeedback(command, strings.NewReader(""), &stdout, &stderr, nil); code != 0 {
			t.Fatalf("%v: code=%d error=%s", command, code, stderr.String())
		}
		if !json.Valid(stdout.Bytes()) {
			t.Fatalf("invalid result: %s", stdout.String())
		}
	}
	if _, err := os.Stat(filepath.Join(root, "absent")); !os.IsNotExist(err) {
		t.Fatalf("check or skipped capture created directory: %v", err)
	}
	if _, err := loadFeedbackConfig(filepath.Join(root, "missing.json")); err == nil {
		t.Fatal("explicit missing config should fail")
	}
}

func TestFeedbackCaptureReviewAndConfigDoesNotHideHistory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ledger")
	t.Setenv("PAPERCLIP_FEEDBACK_HOME", root)
	t.Setenv("PAPERCLIP_HARNESS", "codex")
	t.Setenv("PAPERCLIP_SESSION_ID", "environment-session")
	t.Setenv("PAPERCLIP_SESSION_PATH", "/missing/transcript.jsonl")
	application := app.New(ledger.New(filepath.Join(root, "PAPERCLIP.md")), cliContext{})
	var stdout, stderr bytes.Buffer
	input := `{"skill":"custom:skill","note":"Succeeded, but needed a second lookup.\nInclude IDs in the first result.","session_id":"explicit-session","harness":"","suggestion":"Include IDs","workaround":"Repeated lookup"}`
	code := RunFeedback([]string{"add", "--input-json", "-"}, strings.NewReader(input), &stdout, &stderr, application)
	if code != 0 {
		t.Fatalf("capture: %d %s", code, stderr.String())
	}
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"excludes":["custom:skill"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	code = RunFeedback([]string{"list", "--json"}, strings.NewReader(""), &stdout, &stderr, application)
	var observations []domain.Observation
	if code != 0 || json.Unmarshal(stdout.Bytes(), &observations) != nil || len(observations) != 1 {
		t.Fatalf("list: %d %s %s", code, stdout.String(), stderr.String())
	}
	conversation := observations[0].Conversations[0]
	if conversation.Harness != "" || conversation.SessionID != "explicit-session" || conversation.SessionPath != "/missing/transcript.jsonl" {
		t.Fatalf("explicit input must override defaults: %+v", conversation)
	}
	stdout.Reset()
	code = RunFeedback([]string{"review", "--skill", "custom:skill"}, strings.NewReader(""), &stdout, &stderr, application)
	if code != 0 || !strings.Contains(stdout.String(), "Succeeded, but") || !strings.Contains(stdout.String(), "explicit-session") || !strings.Contains(stdout.String(), "Repeated lookup") {
		t.Fatalf("review lost content: %d %s", code, stdout.String())
	}
}

func TestFeedbackRejectsInvalidInputWithoutWrites(t *testing.T) {
	t.Setenv("PAPERCLIP_FEEDBACK_HOME", filepath.Join(t.TempDir(), "absent"))
	for _, example := range []struct {
		input string
		args  []string
	}{
		{`{"skill":"relay","note":"ok","typo":true}`, []string{"add", "--input-json", "-"}},
		{`{"skill":"relay","note":"ok"} {}`, []string{"add", "--input-json", "-"}},
		{`{"skill":"relay","note":"ok"}`, []string{"add", "--input-json", "-", "--harness", "codex"}},
		{"", []string{"add", "--skill", "relay"}},
		{"", []string{"add", "--skill", "relay", "--note", "ok", "--severity", "wrong"}},
		{"", []string{"add", "--skill", "relay", "--note", "ok", "extra"}},
	} {
		var stdout, stderr bytes.Buffer
		if code := RunFeedback(example.args, strings.NewReader(example.input), &stdout, &stderr, nil); code != ExitUsage {
			t.Fatalf("%v: code=%d error=%s", example.args, code, stderr.String())
		}
	}
}

func TestGeneralCaptureConversationFlagsAndJSON(t *testing.T) {
	for _, jsonInput := range []bool{false, true} {
		root := filepath.Join(t.TempDir(), "ledger")
		t.Setenv("PAPERCLIP_SESSION_ID", "fallback")
		application := app.New(ledger.New(filepath.Join(root, "PAPERCLIP.md")), cliContext{})
		args := []string{"add", "--expected", "success", "--observed", "failure", "--impact", "retry", "--locus", "process", "--session-id", "explicit"}
		input := ""
		if jsonInput {
			args = []string{"add", "--input-json", "-"}
			input = `{"expected":"success","observed":"failure","impact":"retry","locus":"process","session_id":"explicit"}`
		}
		var stdout, stderr bytes.Buffer
		if code := Run(args, strings.NewReader(input), &stdout, &stderr, application); code != 0 {
			t.Fatalf("add: %d %s", code, stderr.String())
		}
		observations, err := application.List(app.QueryOptions{Repo: "all"})
		if err != nil || len(observations) != 1 || len(observations[0].Conversations) != 1 || observations[0].Conversations[0].SessionID != "explicit" {
			t.Fatalf("references lost: %+v %v", observations, err)
		}
	}
}
