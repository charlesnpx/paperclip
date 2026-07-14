package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charlesnpx/paperclip/internal/domain"
	"github.com/charlesnpx/paperclip/internal/ledger"
	"github.com/charlesnpx/paperclip/internal/policy"
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
	application := New(repo, fixedContext{ctx: domain.Context{RepoID: "repo-0123456789abcdef"}}, policy.DefaultScanner()).
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
	application := New(repo, fixedContext{ctx: domain.Context{RepoID: "none"}}, policy.DefaultScanner()).
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

func TestAddRejectsQuotedCredentialsBeforeLedgerWrite(t *testing.T) {
	dir := t.TempDir()
	repo := ledger.New(filepath.Join(dir, "Papercuts", "PAPERCUTS.md"))
	application := New(repo, fixedContext{ctx: domain.Context{RepoID: "none"}}, policy.DefaultScanner())
	for _, observed := range []string{
		`{"authorization":"sensitive-value"}`,
		`password=" CorrectHorseBatteryStaple"`,
		`"authorization":" Basic dXNlcjpwYXNz"`,
		`{\"accessToken\":\"secret\"}`,
		`{"accessToken":"sensitive-value"}`,
		"https://example.invalid/api?accessToken[0]=sensitive-value",
		"https://example.invalid/api?auth[token]=sensitive-value",
		"https://example.invalid/api?credentials[0][accessToken]=sensitive-value",
		"https://example.invalid/api?credentials[clientSecret]=sensitive-value",
		"https://example.invalid/api?headers[authorization]=sensitive-value",
		"https://example.invalid/api?state[session]=sensitive-value",
		"https://x.invalid/p?safe=1;credentials%5BaccessToken%5D=opaque-credential-value",
		"https://example.invalid/%ZZ?access%5Ftoken=opaque-credential-value",
		`password="CorrectHorseBatteryStaple"`,
		"https://example.invalid/api?password=CorrectHorseBatteryStaple",
	} {
		_, err := application.Add(domain.RawRequest{Expected: "e", Observed: observed, Impact: "i", Locus: "repo"})
		if err == nil || !errors.Is(err, ErrPolicy) {
			t.Fatalf("expected policy denial for %q, got %v", observed, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "Papercuts", "PAPERCUTS.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ledger should not be created, stat err=%v", err)
	}
}

func TestListRejectsInvalidRepoFilter(t *testing.T) {
	repo := ledger.New(filepath.Join(t.TempDir(), "Papercuts", "PAPERCUTS.md"))
	application := New(repo, fixedContext{ctx: domain.Context{RepoID: "none"}}, policy.DefaultScanner())
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
	application := New(repo, fixedContext{ctx: domain.Context{RepoID: "none"}}, policy.DefaultScanner()).
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
