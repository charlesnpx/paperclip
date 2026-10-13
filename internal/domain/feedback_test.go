package domain

import (
	"testing"
	"time"
)

func TestFeedbackSchemaAndSnapshotIsolation(t *testing.T) {
	feedback := Feedback{Skill: "custom", Note: "A concrete suggestion"}
	observation := NormalizedObservation{Feedback: &feedback, Severity: SeverityInfo, RepoID: "none"}
	opened, err := NewEvent("event-1", time.Now(), EventFeedbackOpened, "observation-1", "", Context{RepoID: "none"}, FeedbackOpenedPayload{
		Feedback: feedback, Severity: SeverityInfo, Fingerprint: Fingerprint(observation),
	})
	if err != nil || opened.Schema != SchemaVersion2 {
		t.Fatalf("feedback requires v2: %+v %v", opened, err)
	}
	conversation := Conversation{Harness: "future-harness", SessionID: "session-1"}
	linked, err := NewEvent("event-2", time.Now(), EventConversationLinked, "observation-1", "", Context{RepoID: "none"}, conversation)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := ApplyEvents([]Event{opened, linked})
	if err != nil {
		t.Fatal(err)
	}
	copy, _ := snapshot.Observation("observation-1")
	copy.Feedback.Note = "mutated"
	copy.Conversations[0].SessionID = "mutated"
	stored, _ := snapshot.Observation("observation-1")
	if stored.Feedback.Note != feedback.Note || stored.Conversations[0] != conversation {
		t.Fatal("snapshot leaked mutable references")
	}
	opened.Schema = SchemaVersion
	if _, err := ApplyEvents([]Event{opened}); err == nil {
		t.Fatal("v2-only payload accepted as v1")
	}
	opened.Schema = SchemaVersion2
	linked.Context.RepoID = "git-local:no-remote"
	if _, err := ApplyEvents([]Event{opened, linked}); err == nil {
		t.Fatal("mismatched repository accepted")
	}
}
