package review

import (
	"strings"
	"testing"
	"time"

	"github.com/charlesnpx/paperclip/internal/domain"
)

func TestGroupsDeterministically(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	observations := []domain.Observation{
		{ID: "obs_low", CreatedAt: now, State: domain.StateOpen, RepoID: "repo-b", Locus: domain.LocusRepo, Scope: "", Severity: domain.SeverityLow, Expected: "e", Observed: "o", Impact: "i"},
		{ID: "obs_high", CreatedAt: now.Add(time.Second), State: domain.StateOpen, RepoID: "repo-a", Locus: domain.LocusHarness, Scope: "tests", Severity: domain.SeverityHigh, Expected: "e", Observed: "o", Impact: "i", Suggestions: []domain.Suggestion{{Text: "fix"}}},
		{ID: "obs_critical", CreatedAt: now.Add(2 * time.Second), State: domain.StateOpen, RepoID: "repo-a", Locus: domain.LocusHarness, Scope: "tests", Severity: domain.SeverityCritical, Expected: "e", Observed: "o", Impact: "i"},
	}
	groups := Groups(observations)
	if len(groups) != 2 {
		t.Fatalf("groups = %d", len(groups))
	}
	if groups[0].RepoID != "repo-a" || groups[0].Scope != "tests" {
		t.Fatalf("first group = %+v", groups[0])
	}
	if groups[0].Observations[0].ID != "obs_critical" || groups[0].Observations[1].ID != "obs_high" {
		t.Fatalf("group ordering = %+v", groups[0].Observations)
	}
	rendered := RenderReview(groups)
	for _, want := range []string{"Group repo=repo-a locus=harness scope=tests", "- obs_critical [critical]", "Suggestions:"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered review missing %q:\n%s", want, rendered)
		}
	}
}
