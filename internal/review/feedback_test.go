package review

import (
	"strings"
	"testing"

	"github.com/charlesnpx/paperclip/internal/domain"
)

func TestFeedbackGroupsAcrossRepositoriesAndRendersReferences(t *testing.T) {
	observations := []domain.Observation{
		{ID: "first", RepoID: "repo-0123456789abcdef", Scope: "relay:steer", Locus: domain.LocusProcess, Feedback: &domain.Feedback{Skill: "relay:steer", Note: "Useful feedback", Workaround: "Retried"}, Conversations: []domain.Conversation{{Harness: "codex", SessionID: "session-1", SessionPath: "/path/transcript"}}},
		{ID: "second", RepoID: "none", Scope: "relay:steer", Locus: domain.LocusProcess, Feedback: &domain.Feedback{Skill: "relay:steer", Note: "Different feedback"}},
	}
	groups := Groups(observations)
	if len(groups) != 1 || len(groups[0].Observations) != 2 || groups[0].Scope != "relay:steer" {
		t.Fatalf("feedback should group by skill across repos: %+v", groups)
	}
	for _, rendered := range []string{RenderReview(groups), RenderList(observations)} {
		for _, expected := range []string{"Useful feedback", "Retried", "session-1", "/path/transcript", "repo-0123456789abcdef"} {
			if !strings.Contains(rendered, expected) {
				t.Fatalf("missing %s in %s", expected, rendered)
			}
		}
		if strings.Contains(rendered, "Expected:") || strings.Contains(rendered, "expected:") {
			t.Fatalf("feedback must not display synthetic expected/observed fields: %s", rendered)
		}
	}
}
