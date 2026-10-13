package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/charlesnpx/paperclip/internal/domain"
	"github.com/charlesnpx/paperclip/internal/ledger"
)

func TestFeedbackLifecycleAndConversations(t *testing.T) {
	repository := ledger.New(filepath.Join(t.TempDir(), "ledger", "PAPERCLIP.md"))
	application := New(repository, fixedContext{ctx: domain.Context{RepoID: "none"}})
	raw := domain.RawRequest{
		Feedback:     &domain.Feedback{Skill: "relay:steer", Note: "Succeeded after repeating the session ID", Workaround: "Copied the ID", Category: "api"},
		Conversation: domain.Conversation{Harness: "codex", SessionID: "session-1", SessionPath: "/unavailable/transcript.jsonl"},
		Suggestion:   "Remember the session", IdempotencyKey: "first",
	}
	created, err := application.Add(raw)
	if err != nil || !created.Created || created.EventsAppended != 3 {
		t.Fatalf("create: %+v %v", created, err)
	}
	raw.Conversation = domain.Conversation{Harness: "claude", SessionID: "session-2"}
	replay, err := application.Add(raw)
	if err != nil || !replay.Idempotent || replay.EventsAppended != 1 {
		t.Fatalf("replay links new conversation: %+v %v", replay, err)
	}
	replay, err = application.Add(raw)
	if err != nil || replay.EventsAppended != 0 {
		t.Fatalf("same conversation must not append: %+v %v", replay, err)
	}
	raw.IdempotencyKey = "second"
	raw.Suggestion = "Also print the accepted session"
	raw.SessionID = "session-3"
	duplicate, err := application.Add(raw)
	if err != nil || !duplicate.Duplicate || !duplicate.Suggested || duplicate.EventsAppended != 2 {
		t.Fatalf("duplicate links and suggests: %+v %v", duplicate, err)
	}
	observations, err := application.List(QueryOptions{Repo: "all", Scope: "relay:steer"})
	if err != nil || len(observations) != 1 {
		t.Fatalf("list: %+v %v", observations, err)
	}
	observation := observations[0]
	if observation.Feedback.Note != raw.Feedback.Note || observation.Expected != "" || observation.Observed != "" || observation.Impact != "" || len(observation.Conversations) != 3 || len(observation.Suggestions) != 2 {
		t.Fatalf("native feedback lost data: %+v", observation)
	}
	raw.Feedback.Note = "different note"
	if _, err := application.Add(raw); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	if _, err := application.VerifyFixed(created.ObservationID); !errors.Is(err, ErrConflict) {
		t.Fatalf("verification without claim should conflict: %v", err)
	}
	if _, err := application.ClaimFixed(created.ObservationID); err != nil {
		t.Fatal(err)
	}
	if _, err := application.VerifyFixed(created.ObservationID); err != nil {
		t.Fatal(err)
	}
	observations, err = application.List(QueryOptions{Repo: "all"})
	if err != nil || len(observations) != 0 {
		t.Fatalf("verified feedback should be inactive: %+v %v", observations, err)
	}
}

func TestV1LedgerPreservedWhenLinkingConversations(t *testing.T) {
	repository := ledger.New(filepath.Join(t.TempDir(), "ledger", "PAPERCLIP.md"))
	application := New(repository, fixedContext{ctx: domain.Context{RepoID: "none"}})
	raw := domain.RawRequest{Expected: "success", Observed: "failure", Impact: "retry", Locus: "process", IdempotencyKey: "original"}
	original, err := application.Add(raw)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(repository.Path())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := ledger.Parse(before)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range snapshot.Events() {
		if event.Schema != domain.SchemaVersion {
			t.Fatalf("legacy capture should remain v1: %+v", event)
		}
	}
	legacyObservation, _ := snapshot.Observation(original.ObservationID)
	legacyBinding, _ := snapshot.Idempotency("original")
	raw.Conversation = domain.Conversation{Harness: "cursor", SessionURL: "https://example.invalid/session/1"}
	linked, err := application.Add(raw)
	if err != nil || !linked.Idempotent || linked.EventsAppended != 1 {
		t.Fatalf("link to v1 observation: %+v %v", linked, err)
	}
	after, err := os.ReadFile(repository.Path())
	if err != nil || !bytes.HasPrefix(after, before) {
		t.Fatalf("old ledger bytes must not change: %v", err)
	}
	snapshot, err = ledger.Parse(after)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := snapshot.Observation(original.ObservationID)
	binding, _ := snapshot.Idempotency("original")
	if updated.Fingerprint != legacyObservation.Fingerprint || binding != legacyBinding || len(updated.Conversations) != 1 {
		t.Fatalf("legacy identity changed or reference lost: %+v", updated)
	}
	if _, err := application.Dispose(original.ObservationID, "obsolete"); err != nil {
		t.Fatal(err)
	}
	raw.SessionID = "later-session"
	if result, err := application.Add(raw); err != nil || result.EventsAppended != 1 {
		t.Fatalf("idempotent retry may link a terminal observation: %+v %v", result, err)
	}
}
