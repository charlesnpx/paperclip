package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = "paperclip.event.v1"

type EventType string

const (
	EventObservationOpened    EventType = "observation-opened"
	EventRemediationSuggested EventType = "remediation-suggested"
	EventFixedClaimed         EventType = "fixed-claimed"
	EventFixedVerified        EventType = "fixed-verified"
	EventDisposed             EventType = "disposed"
)

type State string

const (
	StateOpen          State = "open"
	StateFixedClaimed  State = "fixed-claimed"
	StateFixedVerified State = "fixed-verified"
	StateDisposed      State = "disposed"
)

type Locus string

const (
	LocusRepo    Locus = "repo"
	LocusMachine Locus = "machine"
	LocusHarness Locus = "harness"
	LocusModel   Locus = "model"
	LocusService Locus = "service"
	LocusProcess Locus = "process"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

type Context struct {
	RepoID string `json:"repo_id"`
}

type Event struct {
	Schema         string          `json:"schema"`
	EventID        string          `json:"event_id"`
	OccurredAt     time.Time       `json:"occurred_at"`
	Type           EventType       `json:"type"`
	SubjectID      string          `json:"subject_id"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	Context        Context         `json:"context"`
	Payload        json.RawMessage `json:"payload"`
}

type ObservationOpenedPayload struct {
	Expected      string   `json:"expected"`
	Observed      string   `json:"observed"`
	Impact        string   `json:"impact"`
	Locus         Locus    `json:"locus"`
	Severity      Severity `json:"severity"`
	Scope         string   `json:"scope,omitempty"`
	Fingerprint   string   `json:"fingerprint"`
	RequestDigest string   `json:"request_digest,omitempty"`
}

type SuggestionPayload struct {
	Suggestion    string `json:"suggestion,omitempty"`
	RequestDigest string `json:"request_digest,omitempty"`
	BindingOnly   bool   `json:"binding_only,omitempty"`
}

type DisposedPayload struct {
	Reason string `json:"reason"`
}

type EmptyPayload struct{}

type RawRequest struct {
	Expected       string `json:"expected"`
	Observed       string `json:"observed"`
	Impact         string `json:"impact"`
	Locus          string `json:"locus"`
	Severity       string `json:"severity"`
	Scope          string `json:"scope"`
	Suggestion     string `json:"suggestion"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ValidatedRequest struct {
	Expected       string
	Observed       string
	Impact         string
	Locus          string
	Severity       string
	Scope          string
	Suggestion     string
	IdempotencyKey string
}

type ScannedRequest struct {
	Validated ValidatedRequest
}

type NormalizedObservation struct {
	Expected       string
	Observed       string
	Impact         string
	Locus          Locus
	Severity       Severity
	Scope          string
	Suggestion     string
	IdempotencyKey string
	RepoID         string
	Fingerprint    string
	RequestDigest  string
}

type PlannedEvents struct {
	Events []Event
}

type CommitResult struct {
	ObservationID  string   `json:"observation_id,omitempty"`
	EventIDs       []string `json:"event_ids,omitempty"`
	Created        bool     `json:"created"`
	Duplicate      bool     `json:"duplicate"`
	Suggested      bool     `json:"suggested"`
	Idempotent     bool     `json:"idempotent"`
	EventsAppended int      `json:"events_appended"`
}

type Suggestion struct {
	CreatedAt time.Time `json:"created_at"`
	Text      string    `json:"text"`
}

type Observation struct {
	ID             string       `json:"id"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	State          State        `json:"state"`
	RepoID         string       `json:"repo_id"`
	Expected       string       `json:"expected"`
	Observed       string       `json:"observed"`
	Impact         string       `json:"impact"`
	Locus          Locus        `json:"locus"`
	Severity       Severity     `json:"severity"`
	Scope          string       `json:"scope,omitempty"`
	Fingerprint    string       `json:"fingerprint"`
	Suggestions    []Suggestion `json:"suggestions,omitempty"`
	DisposedReason string       `json:"disposed_reason,omitempty"`
}

type IdempotencyRecord struct {
	Key           string
	Digest        string
	ObservationID string
}

func NewEvent(eventID string, occurredAt time.Time, eventType EventType, subjectID string, idempotencyKey string, ctx Context, payload any) (Event, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	if len(body) == 0 {
		body = []byte("{}")
	}
	return Event{
		Schema:         SchemaVersion,
		EventID:        eventID,
		OccurredAt:     occurredAt.UTC(),
		Type:           eventType,
		SubjectID:      subjectID,
		IdempotencyKey: idempotencyKey,
		Context:        ctx,
		Payload:        append([]byte(nil), body...),
	}, nil
}

func (e Event) Clone() Event {
	e.Payload = append([]byte(nil), e.Payload...)
	return e
}

func (e Event) PayloadBytes() []byte {
	return append([]byte(nil), e.Payload...)
}

func (o Observation) Clone() Observation {
	o.Suggestions = append([]Suggestion(nil), o.Suggestions...)
	return o
}

func (o Observation) Active() bool {
	return o.State == StateOpen || o.State == StateFixedClaimed
}

func (o Observation) HasSuggestion(text string) bool {
	for _, suggestion := range o.Suggestions {
		if suggestion.Text == text {
			return true
		}
	}
	return false
}

func NormalizeText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}

func ValidateRawRequest(raw RawRequest) (ValidatedRequest, error) {
	validated := ValidatedRequest{
		Expected:       raw.Expected,
		Observed:       raw.Observed,
		Impact:         raw.Impact,
		Locus:          raw.Locus,
		Severity:       raw.Severity,
		Scope:          raw.Scope,
		Suggestion:     raw.Suggestion,
		IdempotencyKey: raw.IdempotencyKey,
	}
	if NormalizeText(validated.Expected) == "" {
		return ValidatedRequest{}, errors.New("expected is required")
	}
	if NormalizeText(validated.Observed) == "" {
		return ValidatedRequest{}, errors.New("observed is required")
	}
	if NormalizeText(validated.Impact) == "" {
		return ValidatedRequest{}, errors.New("impact is required")
	}
	if !validLocus(Locus(strings.ToLower(strings.TrimSpace(validated.Locus)))) {
		return ValidatedRequest{}, errors.New("locus must be one of repo, machine, harness, model, service, process")
	}
	severity := strings.ToLower(strings.TrimSpace(validated.Severity))
	if severity != "" && !validSeverity(Severity(severity)) {
		return ValidatedRequest{}, errors.New("severity must be one of critical, high, medium, low, info")
	}
	return validated, nil
}

func NormalizeScanned(scanned ScannedRequest, ctx Context) (NormalizedObservation, error) {
	if ctx.RepoID == "" {
		return NormalizedObservation{}, errors.New("repo context is required")
	}
	validated := scanned.Validated
	severity := Severity(strings.ToLower(strings.TrimSpace(validated.Severity)))
	if severity == "" {
		severity = SeverityMedium
	}
	normalized := NormalizedObservation{
		Expected:       NormalizeText(validated.Expected),
		Observed:       NormalizeText(validated.Observed),
		Impact:         NormalizeText(validated.Impact),
		Locus:          Locus(strings.ToLower(strings.TrimSpace(validated.Locus))),
		Severity:       severity,
		Scope:          NormalizeText(validated.Scope),
		Suggestion:     NormalizeText(validated.Suggestion),
		IdempotencyKey: NormalizeText(validated.IdempotencyKey),
		RepoID:         ctx.RepoID,
	}
	normalized.Fingerprint = Fingerprint(normalized)
	normalized.RequestDigest = RequestDigest(normalized)
	return normalized, nil
}

func Fingerprint(obs NormalizedObservation) string {
	body := canonicalObservation{
		Expected: obs.Expected,
		Observed: obs.Observed,
		Impact:   obs.Impact,
		Locus:    string(obs.Locus),
		Severity: string(obs.Severity),
		Scope:    obs.Scope,
		RepoID:   obs.RepoID,
	}
	return "fp-" + digestJSON(body)
}

func RequestDigest(obs NormalizedObservation) string {
	body := canonicalRequest{
		Expected:   obs.Expected,
		Observed:   obs.Observed,
		Impact:     obs.Impact,
		Locus:      string(obs.Locus),
		Severity:   string(obs.Severity),
		Scope:      obs.Scope,
		Suggestion: obs.Suggestion,
		RepoID:     obs.RepoID,
	}
	return "req-" + digestJSON(body)
}

type canonicalObservation struct {
	Expected string `json:"expected"`
	Observed string `json:"observed"`
	Impact   string `json:"impact"`
	Locus    string `json:"locus"`
	Severity string `json:"severity"`
	Scope    string `json:"scope"`
	RepoID   string `json:"repo_id"`
}

type canonicalRequest struct {
	Expected   string `json:"expected"`
	Observed   string `json:"observed"`
	Impact     string `json:"impact"`
	Locus      string `json:"locus"`
	Severity   string `json:"severity"`
	Scope      string `json:"scope"`
	Suggestion string `json:"suggestion"`
	RepoID     string `json:"repo_id"`
}

func digestJSON(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:16])
}

func validLocus(locus Locus) bool {
	switch locus {
	case LocusRepo, LocusMachine, LocusHarness, LocusModel, LocusService, LocusProcess:
		return true
	default:
		return false
	}
}

func validSeverity(severity Severity) bool {
	switch severity {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo:
		return true
	default:
		return false
	}
}

func SeverityRank(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 0
	case SeverityHigh:
		return 1
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 3
	case SeverityInfo:
		return 4
	default:
		return 5
	}
}

func ScopeLabel(scope string) string {
	if scope == "" {
		return "(none)"
	}
	return scope
}

func SortObservations(observations []Observation) {
	sort.SliceStable(observations, func(i, j int) bool {
		left, right := observations[i], observations[j]
		if SeverityRank(left.Severity) != SeverityRank(right.Severity) {
			return SeverityRank(left.Severity) < SeverityRank(right.Severity)
		}
		if left.RepoID != right.RepoID {
			return left.RepoID < right.RepoID
		}
		if left.Locus != right.Locus {
			return left.Locus < right.Locus
		}
		if left.Scope != right.Scope {
			return left.Scope < right.Scope
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.ID < right.ID
	})
}

func DecodeEvent(data []byte) (Event, error) {
	type wire Event
	var decoded wire
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decoded); err != nil {
		return Event{}, err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Event{}, errors.New("trailing JSON content")
	}
	event := Event(decoded).Clone()
	if err := validateEventEnvelope(event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func MarshalEvent(event Event) ([]byte, error) {
	return json.Marshal(event.Clone())
}

func DecodePayload[T any](event Event) (T, error) {
	var payload T
	dec := json.NewDecoder(bytes.NewReader(event.Payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		return payload, err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return payload, errors.New("trailing payload JSON content")
	}
	return payload, nil
}

func RequestDigestFromEvent(event Event) (string, error) {
	switch event.Type {
	case EventObservationOpened:
		payload, err := DecodePayload[ObservationOpenedPayload](event)
		return payload.RequestDigest, err
	case EventRemediationSuggested:
		payload, err := DecodePayload[SuggestionPayload](event)
		return payload.RequestDigest, err
	default:
		return "", nil
	}
}

func validateEventEnvelope(event Event) error {
	if event.Schema != SchemaVersion {
		return fmt.Errorf("unknown schema version")
	}
	if event.EventID == "" {
		return errors.New("event_id is required")
	}
	if event.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}
	if event.OccurredAt.Location() != time.UTC {
		_, offset := event.OccurredAt.Zone()
		if offset != 0 {
			return errors.New("occurred_at must be UTC")
		}
	}
	if event.SubjectID == "" {
		return errors.New("subject_id is required")
	}
	if event.Context.RepoID == "" {
		return errors.New("context.repo_id is required")
	}
	switch event.Type {
	case EventObservationOpened, EventRemediationSuggested, EventFixedClaimed, EventFixedVerified, EventDisposed:
	default:
		return errors.New("unknown event type")
	}
	if len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return errors.New("payload must be valid JSON")
	}
	return nil
}
