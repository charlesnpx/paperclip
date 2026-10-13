package domain

import (
	"errors"
	"strings"
)

const SchemaVersion2 = "paperclip.event.v2"

const (
	EventFeedbackOpened     EventType = "feedback-opened"
	EventConversationLinked EventType = "conversation-linked"
)

type Conversation struct {
	Harness     string `json:"harness,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	SessionPath string `json:"session_path,omitempty"`
	SessionURL  string `json:"session_url,omitempty"`
}

func (conversation Conversation) Normalize() Conversation {
	return Conversation{
		Harness: NormalizeText(conversation.Harness), SessionID: NormalizeText(conversation.SessionID),
		SessionPath: NormalizeText(conversation.SessionPath), SessionURL: NormalizeText(conversation.SessionURL),
	}
}

func (observation Observation) HasConversation(conversation Conversation) bool {
	for _, existing := range observation.Conversations {
		if existing == conversation {
			return true
		}
	}
	return false
}

type Feedback struct {
	Skill      string `json:"skill"`
	Note       string `json:"note"`
	Category   string `json:"category,omitempty"`
	Workaround string `json:"workaround,omitempty"`
	Source     string `json:"source,omitempty"`
	Version    string `json:"version,omitempty"`
}

func ValidateSkillName(name string) error {
	if name == "" || strings.ContainsAny(name, " \t\r\n") {
		return errors.New("skill must be a nonempty name without whitespace")
	}
	return nil
}

func (feedback Feedback) Normalize() Feedback {
	return Feedback{
		Skill: NormalizeText(feedback.Skill), Note: NormalizeText(feedback.Note),
		Category: NormalizeText(feedback.Category), Workaround: NormalizeText(feedback.Workaround),
		Source: NormalizeText(feedback.Source), Version: NormalizeText(feedback.Version),
	}
}

func (feedback Feedback) Validate() error {
	if err := ValidateSkillName(feedback.Skill); err != nil {
		return err
	}
	if feedback.Note == "" {
		return errors.New("note is required")
	}
	return nil
}

type FeedbackOpenedPayload struct {
	Feedback    Feedback `json:"feedback"`
	Severity    Severity `json:"severity"`
	Fingerprint string   `json:"fingerprint"`
}

func (snapshot *Snapshot) applyFeedback(event Event) error {
	payload, err := DecodePayload[FeedbackOpenedPayload](event)
	if err != nil {
		return err
	}
	if err := payload.Feedback.Validate(); err != nil {
		return err
	}
	if !validSeverity(payload.Severity) {
		return errors.New("invalid severity")
	}
	if _, exists := snapshot.observations[event.SubjectID]; exists {
		return errors.New("observation already exists")
	}
	normalized := NormalizedObservation{Feedback: &payload.Feedback, Severity: payload.Severity, RepoID: event.Context.RepoID}
	if payload.Fingerprint != Fingerprint(normalized) {
		return errors.New("fingerprint does not match feedback payload")
	}
	snapshot.observations[event.SubjectID] = Observation{
		ID: event.SubjectID, CreatedAt: event.OccurredAt, UpdatedAt: event.OccurredAt,
		State: StateOpen, RepoID: event.Context.RepoID, Feedback: &payload.Feedback,
		Scope: payload.Feedback.Skill, Locus: LocusProcess, Severity: payload.Severity, Fingerprint: payload.Fingerprint,
	}
	snapshot.latestByFingerprint[payload.Fingerprint] = event.SubjectID
	return nil
}

func (snapshot *Snapshot) applyConversation(event Event) error {
	conversation, err := DecodePayload[Conversation](event)
	if err != nil {
		return err
	}
	if conversation == (Conversation{}) || conversation != conversation.Normalize() {
		return errors.New("conversation must be nonempty and normalized")
	}
	observation, exists := snapshot.observations[event.SubjectID]
	if !exists || observation.RepoID != event.Context.RepoID {
		return errors.New("conversation references missing observation or mismatched repository")
	}
	if observation.HasConversation(conversation) {
		return errors.New("conversation is already linked")
	}
	observation.Conversations = append(observation.Conversations, conversation)
	observation.UpdatedAt = event.OccurredAt
	snapshot.observations[event.SubjectID] = observation
	return nil
}
