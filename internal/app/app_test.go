package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charlesnpx/paperclip/internal/domain"
	"github.com/charlesnpx/paperclip/internal/ledger"
)

type fixedContext struct {
	ctx domain.Context
}

func (f fixedContext) Current() (domain.Context, error) {
	return f.ctx, nil
}

func TestAddDedupeSuggestionsAndIdempotency(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	ids := []string{"obs_1", "evt_1", "evt_2", "evt_3"}
	next := 0
	application := New(repo, fixedContext{ctx: domain.Context{RepoID: "repo-0123456789abcdef"}}).
		WithClock(func() time.Time { return time.Date(2026, 7, 14, 12, 0, next, 0, time.UTC) }).
		WithIDGenerator(func(prefix string) (string, error) {
			if next >= len(ids) {
				t.Fatalf("unexpected id request for prefix %s", prefix)
			}
			id := ids[next]
			next++
			return id, nil
		})

	raw := domain.RawRequest{
		Expected:       "tests pass",
		Observed:       "tests fail",
		Impact:         "blocks verification",
		Locus:          "repo",
		Severity:       "high",
		Suggestion:     "capture stderr",
		IdempotencyKey: "idem-1",
	}
	result, err := application.Add(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || !result.Suggested || result.ObservationID != "obs_1" || result.EventsAppended != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	replay, err := application.Add(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Idempotent || replay.ObservationID != "obs_1" || replay.EventsAppended != 0 {
		t.Fatalf("unexpected idempotent replay: %+v", replay)
	}
	raw.IdempotencyKey = ""
	raw.Suggestion = "capture stderr"
	duplicate, err := application.Add(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Duplicate || duplicate.EventsAppended != 0 {
		t.Fatalf("unexpected duplicate result: %+v", duplicate)
	}
	raw.Suggestion = "print compiler command"
	novelSuggestion, err := application.Add(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !novelSuggestion.Duplicate || !novelSuggestion.Suggested || novelSuggestion.EventsAppended != 1 {
		t.Fatalf("unexpected novel suggestion result: %+v", novelSuggestion)
	}
	conflict := raw
	conflict.IdempotencyKey = "idem-1"
	conflict.Observed = "different failure"
	_, err = application.Add(conflict)
	if err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestTransitions(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	ids := []string{"obs_1", "evt_open", "evt_claim", "evt_verify"}
	next := 0
	application := New(repo, fixedContext{ctx: domain.Context{RepoID: "none"}}).
		WithClock(func() time.Time { return time.Date(2026, 7, 14, 12, 0, next, 0, time.UTC) }).
		WithIDGenerator(func(prefix string) (string, error) {
			id := ids[next]
			next++
			return id, nil
		})
	added, err := application.Add(domain.RawRequest{Expected: "e", Observed: "o", Impact: "i", Locus: "harness"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.ClaimFixed(added.ObservationID); err != nil {
		t.Fatal(err)
	}
	if _, err := application.VerifyFixed(added.ObservationID); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Dispose(added.ObservationID, "obsolete"); err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("expected terminal transition conflict, got %v", err)
	}
}

func TestAddStoresLiteralUserTextWithoutContentScanning(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	application := New(repo, fixedContext{ctx: domain.Context{RepoID: "none"}})
	observed := "fetch failed for https://build:placeholder@registry.example/package with token=ghp_abcdefghijklmnopqrstuvwxyz123456"

	result, err := application.Add(domain.RawRequest{Expected: "e", Observed: observed, Impact: "i", Locus: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created {
		t.Fatalf("expected created observation, got %+v", result)
	}
	observations, err := application.List(QueryOptions{Repo: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || observations[0].Observed != observed {
		t.Fatalf("literal observed text was not preserved: %+v", observations)
	}
	if !strings.Contains(observations[0].Observed, "ghp_abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("expected stored text to include literal token-like value: %q", observations[0].Observed)
	}
}

func TestListRejectsInvalidRepoFilter(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	application := New(repo, fixedContext{ctx: domain.Context{RepoID: "none"}})
	if _, err := application.List(QueryOptions{Repo: "curent"}); err == nil || !errors.Is(err, ErrUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if _, err := application.List(QueryOptions{Repo: "repo-0123456789abcdef"}); err != nil {
		t.Fatalf("valid repo filter rejected: %v", err)
	}
}

func TestDuplicateCanPersistIdempotencyBindingWithoutSuggestion(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	ids := []string{"obs_1", "evt_open", "evt_bind"}
	next := 0
	application := New(repo, fixedContext{ctx: domain.Context{RepoID: "none"}}).
		WithClock(func() time.Time { return time.Date(2026, 7, 14, 12, 0, next, 0, time.UTC) }).
		WithIDGenerator(func(prefix string) (string, error) {
			id := ids[next]
			next++
			return id, nil
		})
	raw := domain.RawRequest{Expected: "e", Observed: "o", Impact: "i", Locus: "repo"}
	added, err := application.Add(raw)
	if err != nil {
		t.Fatal(err)
	}
	if added.ObservationID != "obs_1" {
		t.Fatalf("added = %+v", added)
	}
	withKey := raw
	withKey.IdempotencyKey = "idem-bind"
	bound, err := application.Add(withKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bound.Duplicate || bound.EventsAppended != 1 {
		t.Fatalf("expected duplicate binding event, got %+v", bound)
	}
	replay, err := application.Add(withKey)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Idempotent || replay.ObservationID != "obs_1" {
		t.Fatalf("expected idempotent replay, got %+v", replay)
	}
	conflict := withKey
	conflict.Observed = "different"
	if _, err := application.Add(conflict); err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict after binding, got %v", err)
	}
	observations, err := application.List(QueryOptions{Repo: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || len(observations[0].Suggestions) != 0 {
		t.Fatalf("binding event should not create visible suggestion: %+v", observations)
	}
}
