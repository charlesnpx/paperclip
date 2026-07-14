package domain

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var repoIDPattern = regexp.MustCompile(`^(none|git-local:no-remote|repo-[0-9a-f]{16})$`)

type Snapshot struct {
	events              []Event
	observations        map[string]Observation
	activeByFingerprint map[string]string
	idempotency         map[string]IdempotencyRecord
}

func EmptySnapshot() Snapshot {
	return Snapshot{
		observations:        map[string]Observation{},
		activeByFingerprint: map[string]string{},
		idempotency:         map[string]IdempotencyRecord{},
	}
}

func ApplyEvents(events []Event) (Snapshot, error) {
	snapshot := EmptySnapshot()
	seenEvents := map[string]struct{}{}
	for idx, event := range events {
		event = event.Clone()
		if err := validateEventEnvelope(event); err != nil {
			return Snapshot{}, fmt.Errorf("event %d: %w", idx+1, err)
		}
		if !repoIDPattern.MatchString(event.Context.RepoID) {
			return Snapshot{}, fmt.Errorf("event %d: context.repo_id is not sanitized", idx+1)
		}
		if _, ok := seenEvents[event.EventID]; ok {
			return Snapshot{}, fmt.Errorf("event %d: duplicate event_id", idx+1)
		}
		seenEvents[event.EventID] = struct{}{}
		if err := snapshot.apply(event); err != nil {
			return Snapshot{}, fmt.Errorf("event %d: %w", idx+1, err)
		}
		snapshot.events = append(snapshot.events, event.Clone())
	}
	return snapshot, nil
}

func (s *Snapshot) apply(event Event) error {
	if event.IdempotencyKey != "" {
		digest, err := RequestDigestFromEvent(event)
		if err != nil {
			return err
		}
		if digest == "" {
			return errors.New("idempotency_key requires request_digest")
		}
		record := IdempotencyRecord{Key: event.IdempotencyKey, Digest: digest, ObservationID: event.SubjectID}
		if existing, ok := s.idempotency[event.IdempotencyKey]; ok {
			_ = existing
			return errors.New("duplicate idempotency_key in ledger")
		}
		s.idempotency[event.IdempotencyKey] = record
	}

	switch event.Type {
	case EventObservationOpened:
		payload, err := DecodePayload[ObservationOpenedPayload](event)
		if err != nil {
			return err
		}
		if _, exists := s.observations[event.SubjectID]; exists {
			return errors.New("observation already exists")
		}
		if payload.Expected == "" || payload.Observed == "" || payload.Impact == "" {
			return errors.New("observation payload requires expected, observed, and impact")
		}
		if !validLocus(payload.Locus) {
			return errors.New("invalid locus")
		}
		if !validSeverity(payload.Severity) {
			return errors.New("invalid severity")
		}
		if payload.Fingerprint == "" {
			return errors.New("fingerprint is required")
		}
		expected := NormalizedObservation{
			Expected: payload.Expected,
			Observed: payload.Observed,
			Impact:   payload.Impact,
			Locus:    payload.Locus,
			Severity: payload.Severity,
			Scope:    payload.Scope,
			RepoID:   event.Context.RepoID,
		}
		if payload.Fingerprint != Fingerprint(expected) {
			return errors.New("fingerprint does not match observation payload")
		}
		if payload.RequestDigest != "" && payload.RequestDigest != RequestDigest(expected) {
			return errors.New("request_digest does not match observation payload")
		}
		obs := Observation{
			ID:          event.SubjectID,
			CreatedAt:   event.OccurredAt,
			UpdatedAt:   event.OccurredAt,
			State:       StateOpen,
			RepoID:      event.Context.RepoID,
			Expected:    payload.Expected,
			Observed:    payload.Observed,
			Impact:      payload.Impact,
			Locus:       payload.Locus,
			Severity:    payload.Severity,
			Scope:       payload.Scope,
			Fingerprint: payload.Fingerprint,
		}
		if existingID, exists := s.activeByFingerprint[obs.Fingerprint]; exists {
			return fmt.Errorf("duplicate active fingerprint already belongs to %s", existingID)
		}
		s.observations[obs.ID] = obs
		s.activeByFingerprint[obs.Fingerprint] = obs.ID
	case EventRemediationSuggested:
		payload, err := DecodePayload[SuggestionPayload](event)
		if err != nil {
			return err
		}
		obs, ok := s.observations[event.SubjectID]
		if !ok {
			return errors.New("suggestion references missing observation")
		}
		if payload.RequestDigest != "" {
			expected := NormalizedObservation{
				Expected:   obs.Expected,
				Observed:   obs.Observed,
				Impact:     obs.Impact,
				Locus:      obs.Locus,
				Severity:   obs.Severity,
				Scope:      obs.Scope,
				Suggestion: payload.Suggestion,
				RepoID:     obs.RepoID,
			}
			if payload.RequestDigest != RequestDigest(expected) {
				return errors.New("request_digest does not match request payload")
			}
		}
		if payload.BindingOnly {
			if event.IdempotencyKey == "" || payload.RequestDigest == "" {
				return errors.New("idempotency binding requires key and request_digest")
			}
			return nil
		}
		if payload.Suggestion == "" {
			return errors.New("suggestion is required")
		}
		obs.Suggestions = append(obs.Suggestions, Suggestion{CreatedAt: event.OccurredAt, Text: payload.Suggestion})
		obs.UpdatedAt = event.OccurredAt
		s.observations[event.SubjectID] = obs
	case EventFixedClaimed:
		if !isObjectPayload(event.Payload) {
			return errors.New("payload must be an object")
		}
		if _, err := DecodePayload[EmptyPayload](event); err != nil {
			return err
		}
		obs, ok := s.observations[event.SubjectID]
		if !ok {
			return errors.New("fixed-claimed references missing observation")
		}
		if obs.State != StateOpen {
			return errors.New("invalid transition to fixed-claimed")
		}
		delete(s.activeByFingerprint, obs.Fingerprint)
		obs.State = StateFixedClaimed
		obs.UpdatedAt = event.OccurredAt
		s.observations[event.SubjectID] = obs
		s.activeByFingerprint[obs.Fingerprint] = obs.ID
	case EventFixedVerified:
		if !isObjectPayload(event.Payload) {
			return errors.New("payload must be an object")
		}
		if _, err := DecodePayload[EmptyPayload](event); err != nil {
			return err
		}
		obs, ok := s.observations[event.SubjectID]
		if !ok {
			return errors.New("fixed-verified references missing observation")
		}
		if obs.State != StateFixedClaimed {
			return errors.New("invalid transition to fixed-verified")
		}
		delete(s.activeByFingerprint, obs.Fingerprint)
		obs.State = StateFixedVerified
		obs.UpdatedAt = event.OccurredAt
		s.observations[event.SubjectID] = obs
	case EventDisposed:
		payload, err := DecodePayload[DisposedPayload](event)
		if err != nil {
			return err
		}
		if payload.Reason == "" {
			return errors.New("disposed reason is required")
		}
		obs, ok := s.observations[event.SubjectID]
		if !ok {
			return errors.New("disposed references missing observation")
		}
		if obs.State != StateOpen && obs.State != StateFixedClaimed {
			return errors.New("invalid transition to disposed")
		}
		delete(s.activeByFingerprint, obs.Fingerprint)
		obs.State = StateDisposed
		obs.UpdatedAt = event.OccurredAt
		obs.DisposedReason = payload.Reason
		s.observations[event.SubjectID] = obs
	default:
		return errors.New("unknown event type")
	}
	return nil
}

func isObjectPayload(payload []byte) bool {
	trimmed := bytes.TrimSpace(payload)
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}'
}

func (s Snapshot) Events() []Event {
	out := make([]Event, len(s.events))
	for i, event := range s.events {
		out[i] = event.Clone()
	}
	return out
}

func (s Snapshot) Observations() []Observation {
	out := make([]Observation, 0, len(s.observations))
	for _, obs := range s.observations {
		out = append(out, obs.Clone())
	}
	SortObservations(out)
	return out
}

func (s Snapshot) Observation(id string) (Observation, bool) {
	obs, ok := s.observations[id]
	if !ok {
		return Observation{}, false
	}
	return obs.Clone(), true
}

func (s Snapshot) ActiveObservationByFingerprint(fingerprint string) (Observation, bool) {
	id, ok := s.activeByFingerprint[fingerprint]
	if !ok {
		return Observation{}, false
	}
	return s.Observation(id)
}

func (s Snapshot) Idempotency(key string) (IdempotencyRecord, bool) {
	record, ok := s.idempotency[key]
	return record, ok
}

func FilterObservations(observations []Observation, repoID string, locus string, scope string, activeOnly bool) ([]Observation, error) {
	var normalizedLocus Locus
	if locus != "" {
		normalizedLocus = Locus(strings.ToLower(strings.TrimSpace(locus)))
		if !validLocus(normalizedLocus) {
			return nil, errors.New("invalid locus filter")
		}
	}
	normalizedScope := NormalizeText(scope)
	out := make([]Observation, 0, len(observations))
	for _, obs := range observations {
		if activeOnly && !obs.Active() {
			continue
		}
		if repoID != "all" && obs.RepoID != repoID {
			continue
		}
		if normalizedLocus != "" && obs.Locus != normalizedLocus {
			continue
		}
		if scope != "" && obs.Scope != normalizedScope {
			continue
		}
		out = append(out, obs.Clone())
	}
	SortObservations(out)
	return out, nil
}

func IdempotencyRecords(snapshot Snapshot) []IdempotencyRecord {
	out := make([]IdempotencyRecord, 0, len(snapshot.idempotency))
	for _, record := range snapshot.idempotency {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
