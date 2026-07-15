package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/charlesnpx/paperclip/internal/domain"
	"github.com/charlesnpx/paperclip/internal/ledger"
)

const DefaultLockTimeout = 5 * time.Second

type Store interface {
	Read(timeout time.Duration) (domain.Snapshot, error)
	Commit(timeout time.Duration, plan func(domain.Snapshot) ([]domain.Event, domain.CommitResult, error)) (domain.CommitResult, error)
}

type ContextResolver interface {
	Current() (domain.Context, error)
}

type App struct {
	store       Store
	context     ContextResolver
	clock       func() time.Time
	newID       func(prefix string) (string, error)
	lockTimeout time.Duration
}

func New(store Store, context ContextResolver) *App {
	return &App{
		store:       store,
		context:     context,
		clock:       func() time.Time { return time.Now().UTC() },
		newID:       randomID,
		lockTimeout: DefaultLockTimeout,
	}
}

func (a *App) WithClock(clock func() time.Time) *App {
	cp := *a
	cp.clock = clock
	return &cp
}

func (a *App) WithIDGenerator(newID func(prefix string) (string, error)) *App {
	cp := *a
	cp.newID = newID
	return &cp
}

func (a *App) WithLockTimeout(timeout time.Duration) *App {
	cp := *a
	cp.lockTimeout = timeout
	return &cp
}

func (a *App) Add(raw domain.RawRequest) (domain.CommitResult, error) {
	validated, err := domain.ValidateRawRequest(raw)
	if err != nil {
		return domain.CommitResult{}, fmt.Errorf("%w: %v", ErrUsage, err)
	}
	scanned := domain.ScannedRequest{Validated: validated}
	ctx, err := a.context.Current()
	if err != nil {
		return domain.CommitResult{}, err
	}
	normalized, err := domain.NormalizeScanned(scanned, ctx)
	if err != nil {
		return domain.CommitResult{}, fmt.Errorf("%w: %v", ErrUsage, err)
	}
	return a.store.Commit(a.lockTimeout, func(snapshot domain.Snapshot) ([]domain.Event, domain.CommitResult, error) {
		if normalized.IdempotencyKey != "" {
			record, ok := snapshot.Idempotency(normalized.IdempotencyKey)
			if ok {
				if record.Digest != normalized.RequestDigest {
					return nil, domain.CommitResult{}, fmt.Errorf("%w: idempotency key was reused with different content", ErrConflict)
				}
				return nil, domain.CommitResult{ObservationID: record.ObservationID, Idempotent: true}, nil
			}
		}

		if existing, ok := snapshot.ActiveObservationByFingerprint(normalized.Fingerprint); ok {
			var events []domain.Event
			result := domain.CommitResult{ObservationID: existing.ID, Duplicate: true}
			if normalized.Suggestion != "" && !existing.HasSuggestion(normalized.Suggestion) {
				event, err := a.newSuggestionEvent(existing.ID, normalized)
				if err != nil {
					return nil, domain.CommitResult{}, err
				}
				events = append(events, event)
				result.Suggested = true
			}
			if normalized.IdempotencyKey != "" && len(events) == 0 {
				event, err := a.newIdempotencyBindingEvent(existing.ID, normalized)
				if err != nil {
					return nil, domain.CommitResult{}, err
				}
				events = append(events, event)
			}
			return events, result, nil
		}

		obsID, err := a.newID("obs")
		if err != nil {
			return nil, domain.CommitResult{}, err
		}
		openEvent, err := a.newOpenEvent(obsID, normalized)
		if err != nil {
			return nil, domain.CommitResult{}, err
		}
		events := []domain.Event{openEvent}
		result := domain.CommitResult{ObservationID: obsID, Created: true}
		if normalized.Suggestion != "" {
			suggestion, err := a.newSuggestionEvent(obsID, normalized)
			if err != nil {
				return nil, domain.CommitResult{}, err
			}
			events = append(events, suggestion)
			result.Suggested = true
		} else if normalized.IdempotencyKey != "" {
			binding, err := a.newIdempotencyBindingEvent(obsID, normalized)
			if err != nil {
				return nil, domain.CommitResult{}, err
			}
			events = append(events, binding)
		}
		return events, result, nil
	})
}

func (a *App) ClaimFixed(id string) (domain.CommitResult, error) {
	return a.transition(id, domain.EventFixedClaimed, domain.EmptyPayload{})
}

func (a *App) VerifyFixed(id string) (domain.CommitResult, error) {
	return a.transition(id, domain.EventFixedVerified, domain.EmptyPayload{})
}

func (a *App) Dispose(id string, reason string) (domain.CommitResult, error) {
	reason = domain.NormalizeText(reason)
	if reason == "" {
		return domain.CommitResult{}, fmt.Errorf("%w: reason is required", ErrUsage)
	}
	return a.transition(id, domain.EventDisposed, domain.DisposedPayload{Reason: reason})
}

func (a *App) transition(id string, eventType domain.EventType, payload any) (domain.CommitResult, error) {
	id = domain.NormalizeText(id)
	if id == "" {
		return domain.CommitResult{}, fmt.Errorf("%w: observation id is required", ErrUsage)
	}
	ctx, err := a.context.Current()
	if err != nil {
		return domain.CommitResult{}, err
	}
	return a.store.Commit(a.lockTimeout, func(snapshot domain.Snapshot) ([]domain.Event, domain.CommitResult, error) {
		obs, ok := snapshot.Observation(id)
		if !ok {
			return nil, domain.CommitResult{}, fmt.Errorf("%w: observation not found", ErrConflict)
		}
		if err := validateTransition(obs.State, eventType); err != nil {
			return nil, domain.CommitResult{}, err
		}
		eventID, err := a.newID("evt")
		if err != nil {
			return nil, domain.CommitResult{}, err
		}
		event, err := domain.NewEvent(eventID, a.clock(), eventType, id, "", ctx, payload)
		if err != nil {
			return nil, domain.CommitResult{}, err
		}
		return []domain.Event{event}, domain.CommitResult{ObservationID: id}, nil
	})
}

func validateTransition(state domain.State, eventType domain.EventType) error {
	switch eventType {
	case domain.EventFixedClaimed:
		if state == domain.StateOpen {
			return nil
		}
	case domain.EventFixedVerified:
		if state == domain.StateFixedClaimed {
			return nil
		}
	case domain.EventDisposed:
		if state == domain.StateOpen || state == domain.StateFixedClaimed {
			return nil
		}
	}
	return fmt.Errorf("%w: invalid lifecycle transition", ErrConflict)
}

type RepoFilter string

const (
	RepoFilterCurrent RepoFilter = "current"
	RepoFilterAll     RepoFilter = "all"
	RepoFilterNone    RepoFilter = "none"
)

type QueryOptions struct {
	Repo  string
	Locus string
	Scope string
}

func (a *App) List(opts QueryOptions) ([]domain.Observation, error) {
	repoID, err := a.resolveRepoFilter(opts.Repo)
	if err != nil {
		return nil, err
	}
	snapshot, err := a.store.Read(a.lockTimeout)
	if err != nil {
		return nil, err
	}
	observations, err := domain.FilterObservations(snapshot.Observations(), repoID, opts.Locus, opts.Scope, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUsage, err)
	}
	return observations, nil
}

func (a *App) resolveRepoFilter(filter string) (string, error) {
	if filter == "" {
		filter = string(RepoFilterCurrent)
	}
	switch RepoFilter(filter) {
	case RepoFilterAll:
		return "all", nil
	case RepoFilterNone:
		return "none", nil
	case RepoFilterCurrent:
		ctx, err := a.context.Current()
		if err != nil {
			return "", err
		}
		return ctx.RepoID, nil
	default:
		if filter == "" {
			return "", fmt.Errorf("%w: repo filter is required", ErrUsage)
		}
		if !validRepoFilter.MatchString(filter) {
			return "", fmt.Errorf("%w: invalid repo filter", ErrUsage)
		}
		return filter, nil
	}
}

var validRepoFilter = regexp.MustCompile(`^(git-local:no-remote|repo-[0-9a-f]{16})$`)

func (a *App) newOpenEvent(obsID string, obs domain.NormalizedObservation) (domain.Event, error) {
	eventID, err := a.newID("evt")
	if err != nil {
		return domain.Event{}, err
	}
	return domain.NewEvent(eventID, a.clock(), domain.EventObservationOpened, obsID, "", domain.Context{RepoID: obs.RepoID}, domain.ObservationOpenedPayload{
		Expected:    obs.Expected,
		Observed:    obs.Observed,
		Impact:      obs.Impact,
		Locus:       obs.Locus,
		Severity:    obs.Severity,
		Scope:       obs.Scope,
		Fingerprint: obs.Fingerprint,
	})
}

func (a *App) newSuggestionEvent(obsID string, obs domain.NormalizedObservation) (domain.Event, error) {
	eventID, err := a.newID("evt")
	if err != nil {
		return domain.Event{}, err
	}
	return domain.NewEvent(eventID, a.clock(), domain.EventRemediationSuggested, obsID, obs.IdempotencyKey, domain.Context{RepoID: obs.RepoID}, domain.SuggestionPayload{
		Suggestion:    obs.Suggestion,
		RequestDigest: obs.RequestDigest,
	})
}

func (a *App) newIdempotencyBindingEvent(obsID string, obs domain.NormalizedObservation) (domain.Event, error) {
	eventID, err := a.newID("evt")
	if err != nil {
		return domain.Event{}, err
	}
	return domain.NewEvent(eventID, a.clock(), domain.EventRemediationSuggested, obsID, obs.IdempotencyKey, domain.Context{RepoID: obs.RepoID}, domain.SuggestionPayload{
		RequestDigest: obs.RequestDigest,
		BindingOnly:   true,
		Suggestion:    obs.Suggestion,
	})
}

func randomID(prefix string) (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(buf), nil
}

func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, ErrUsage):
		return 2
	case errors.Is(err, ErrConflict):
		return 4
	case ledger.IsMalformed(err):
		return 5
	case ledger.IsLockTimeout(err):
		return 6
	default:
		return 1
	}
}
