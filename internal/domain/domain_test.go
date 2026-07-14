package domain

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeAndFingerprint(t *testing.T) {
	ctx := Context{RepoID: "repo-abc"}
	validated, err := ValidateRawRequest(RawRequest{
		Expected:   "  Keep CRLF\r\nstable  ",
		Observed:   "Fails",
		Impact:     "Blocks work",
		Locus:      "REPO",
		Severity:   "",
		Scope:      " Tests ",
		Suggestion: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	one, err := NormalizeScanned(ScannedRequest{Validated: validated}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	validated.Suggestion = "second"
	two, err := NormalizeScanned(ScannedRequest{Validated: validated}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if one.Expected != "Keep CRLF\nstable" {
		t.Fatalf("expected normalized CRLF, got %q", one.Expected)
	}
	if one.Severity != SeverityMedium {
		t.Fatalf("default severity = %q", one.Severity)
	}
	if one.Fingerprint != two.Fingerprint {
		t.Fatalf("fingerprint should exclude suggestion")
	}
	if one.RequestDigest == two.RequestDigest {
		t.Fatalf("request digest should include suggestion")
	}
}

func TestApplyEventsTransitionsAndTerminal(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	payload := openedPayload("expected", "observed", "impact", LocusHarness, SeverityHigh, "", "none")
	open := mustEvent(t, "evt_open", now, EventObservationOpened, "obs_1", "", Context{RepoID: "none"}, ObservationOpenedPayload{
		Expected:    payload.Expected,
		Observed:    payload.Observed,
		Impact:      payload.Impact,
		Locus:       payload.Locus,
		Severity:    payload.Severity,
		Fingerprint: payload.Fingerprint,
	})
	suggestion := mustEvent(t, "evt_suggestion", now.Add(time.Second), EventRemediationSuggested, "obs_1", "", Context{RepoID: "none"}, SuggestionPayload{
		Suggestion: "try this",
	})
	claimed := mustEvent(t, "evt_claimed", now.Add(2*time.Second), EventFixedClaimed, "obs_1", "", Context{RepoID: "none"}, EmptyPayload{})
	verified := mustEvent(t, "evt_verified", now.Add(3*time.Second), EventFixedVerified, "obs_1", "", Context{RepoID: "none"}, EmptyPayload{})

	snapshot, err := ApplyEvents([]Event{open, suggestion, claimed, verified})
	if err != nil {
		t.Fatal(err)
	}
	obs, ok := snapshot.Observation("obs_1")
	if !ok {
		t.Fatal("observation missing")
	}
	if obs.State != StateFixedVerified {
		t.Fatalf("state = %q", obs.State)
	}
	if len(obs.Suggestions) != 1 {
		t.Fatalf("suggestions = %d", len(obs.Suggestions))
	}

	dispose := mustEvent(t, "evt_dispose", now.Add(4*time.Second), EventDisposed, "obs_1", "", Context{RepoID: "none"}, DisposedPayload{Reason: "done"})
	if _, err := ApplyEvents([]Event{open, claimed, verified, dispose}); err == nil || !strings.Contains(err.Error(), "invalid transition") {
		t.Fatalf("expected terminal transition rejection, got %v", err)
	}
}

func TestApplyEventsRejectsStaleDerivedFields(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	normalized := NormalizedObservation{
		Expected: "expected",
		Observed: "observed",
		Impact:   "impact",
		Locus:    LocusRepo,
		Severity: SeverityHigh,
		RepoID:   "none",
	}
	normalized.Fingerprint = Fingerprint(normalized)
	open := mustEvent(t, "evt_open", now, EventObservationOpened, "obs_1", "", Context{RepoID: "none"}, ObservationOpenedPayload{
		Expected:    "tampered",
		Observed:    normalized.Observed,
		Impact:      normalized.Impact,
		Locus:       normalized.Locus,
		Severity:    normalized.Severity,
		Fingerprint: normalized.Fingerprint,
	})
	if _, err := ApplyEvents([]Event{open}); err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("expected stale fingerprint rejection, got %v", err)
	}

	normalized.Fingerprint = Fingerprint(normalized)
	normalized.Suggestion = "first"
	normalized.RequestDigest = RequestDigest(normalized)
	validOpen := mustEvent(t, "evt_open_valid", now, EventObservationOpened, "obs_2", "", Context{RepoID: "none"}, ObservationOpenedPayload{
		Expected:    normalized.Expected,
		Observed:    normalized.Observed,
		Impact:      normalized.Impact,
		Locus:       normalized.Locus,
		Severity:    normalized.Severity,
		Fingerprint: normalized.Fingerprint,
	})
	suggestion := mustEvent(t, "evt_suggestion", now.Add(time.Second), EventRemediationSuggested, "obs_2", "idem", Context{RepoID: "none"}, SuggestionPayload{
		Suggestion:    "second",
		RequestDigest: normalized.RequestDigest,
	})
	if _, err := ApplyEvents([]Event{validOpen, suggestion}); err == nil || !strings.Contains(err.Error(), "request_digest") {
		t.Fatalf("expected stale request digest rejection, got %v", err)
	}
}

func TestApplyEventsRejectsDuplicateActiveFingerprints(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	payload := openedPayload("expected", "observed", "impact", LocusRepo, SeverityMedium, "", "none")
	first := mustEvent(t, "evt_1", now, EventObservationOpened, "obs_1", "", Context{RepoID: "none"}, payload)
	second := mustEvent(t, "evt_2", now.Add(time.Second), EventObservationOpened, "obs_2", "", Context{RepoID: "none"}, payload)
	if _, err := ApplyEvents([]Event{first, second}); err == nil || !strings.Contains(err.Error(), "duplicate active fingerprint") {
		t.Fatalf("expected duplicate active fingerprint rejection, got %v", err)
	}
}

func TestApplyEventsRejectsDuplicateIdempotencyKey(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	normalized := NormalizedObservation{
		Expected: "expected",
		Observed: "observed",
		Impact:   "impact",
		Locus:    LocusRepo,
		Severity: SeverityMedium,
		RepoID:   "none",
	}
	normalized.Fingerprint = Fingerprint(normalized)
	normalized.IdempotencyKey = "idem-1"
	normalized.RequestDigest = RequestDigest(normalized)
	open := mustEvent(t, "evt_open", now, EventObservationOpened, "obs_1", "", Context{RepoID: "none"}, ObservationOpenedPayload{
		Expected:    normalized.Expected,
		Observed:    normalized.Observed,
		Impact:      normalized.Impact,
		Locus:       normalized.Locus,
		Severity:    normalized.Severity,
		Fingerprint: normalized.Fingerprint,
	})
	first := mustEvent(t, "evt_bind_1", now.Add(time.Second), EventRemediationSuggested, "obs_1", "idem-1", Context{RepoID: "none"}, SuggestionPayload{
		RequestDigest: normalized.RequestDigest,
		BindingOnly:   true,
	})
	second := mustEvent(t, "evt_bind_2", now.Add(2*time.Second), EventRemediationSuggested, "obs_1", "idem-1", Context{RepoID: "none"}, SuggestionPayload{
		RequestDigest: normalized.RequestDigest,
		BindingOnly:   true,
	})
	if _, err := ApplyEvents([]Event{open, first, second}); err == nil || !strings.Contains(err.Error(), "duplicate idempotency_key") {
		t.Fatalf("expected duplicate idempotency rejection, got %v", err)
	}
}

func TestApplyEventsRejectsUnsanitizedRepoIDs(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	normalized := NormalizedObservation{
		Expected: "expected",
		Observed: "observed",
		Impact:   "impact",
		Locus:    LocusRepo,
		Severity: SeverityMedium,
		RepoID:   "prod.internal",
	}
	open := mustEvent(t, "evt_open", now, EventObservationOpened, "obs_1", "", Context{RepoID: "prod.internal"}, ObservationOpenedPayload{
		Expected:    normalized.Expected,
		Observed:    normalized.Observed,
		Impact:      normalized.Impact,
		Locus:       normalized.Locus,
		Severity:    normalized.Severity,
		Fingerprint: Fingerprint(normalized),
	})
	if _, err := ApplyEvents([]Event{open}); err == nil || !strings.Contains(err.Error(), "context.repo_id") {
		t.Fatalf("expected repo id rejection, got %v", err)
	}
}

func TestApplyEventsRejectsNullLifecyclePayload(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	payload := openedPayload("expected", "observed", "impact", LocusRepo, SeverityMedium, "", "none")
	open := mustEvent(t, "evt_open", now, EventObservationOpened, "obs_1", "", Context{RepoID: "none"}, payload)
	claimed := Event{
		Schema:     SchemaVersion,
		EventID:    "evt_claim",
		OccurredAt: now.Add(time.Second),
		Type:       EventFixedClaimed,
		SubjectID:  "obs_1",
		Context:    Context{RepoID: "none"},
		Payload:    []byte("null"),
	}
	if _, err := ApplyEvents([]Event{open, claimed}); err == nil || !strings.Contains(err.Error(), "payload must be an object") {
		t.Fatalf("expected null payload rejection, got %v", err)
	}
}

func TestSnapshotAndEventCopyBoundaries(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	payload := openedPayload("expected", "observed", "impact", LocusRepo, SeverityLow, "", "none")
	event := mustEvent(t, "evt_open", now, EventObservationOpened, "obs_1", "", Context{RepoID: "none"}, ObservationOpenedPayload{
		Expected:    payload.Expected,
		Observed:    payload.Observed,
		Impact:      payload.Impact,
		Locus:       payload.Locus,
		Severity:    payload.Severity,
		Fingerprint: payload.Fingerprint,
	})
	payloadBytes := event.PayloadBytes()
	payloadBytes[0] = 'x'
	if event.Payload[0] == 'x' {
		t.Fatal("PayloadBytes returned mutable event storage")
	}

	suggestion := mustEvent(t, "evt_suggestion", now.Add(time.Second), EventRemediationSuggested, "obs_1", "", Context{RepoID: "none"}, SuggestionPayload{Suggestion: "fix"})
	snapshot, err := ApplyEvents([]Event{event, suggestion})
	if err != nil {
		t.Fatal(err)
	}
	observations := snapshot.Observations()
	observations[0].Suggestions[0].Text = "mutated"
	again, _ := snapshot.Observation("obs_1")
	if again.Suggestions[0].Text == "mutated" {
		t.Fatal("Observations returned mutable suggestion storage")
	}
	events := snapshot.Events()
	events[0].Payload[0] = 'x'
	againEvents := snapshot.Events()
	if againEvents[0].Payload[0] == 'x' {
		t.Fatal("Events returned mutable payload storage")
	}
}

func mustEvent(t *testing.T, eventID string, at time.Time, typ EventType, subjectID string, idem string, ctx Context, payload any) Event {
	t.Helper()
	event, err := NewEvent(eventID, at, typ, subjectID, idem, ctx, payload)
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func openedPayload(expected string, observed string, impact string, locus Locus, severity Severity, scope string, repoID string) ObservationOpenedPayload {
	normalized := NormalizedObservation{
		Expected: expected,
		Observed: observed,
		Impact:   impact,
		Locus:    locus,
		Severity: severity,
		Scope:    scope,
		RepoID:   repoID,
	}
	return ObservationOpenedPayload{
		Expected:    expected,
		Observed:    observed,
		Impact:      impact,
		Locus:       locus,
		Severity:    severity,
		Scope:       scope,
		Fingerprint: Fingerprint(normalized),
	}
}
