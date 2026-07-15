package review

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/charlesnpx/paperclip/internal/domain"
)

type Group struct {
	RepoID       string               `json:"repo_id"`
	Locus        domain.Locus         `json:"locus"`
	Scope        string               `json:"scope"`
	Observations []domain.Observation `json:"observations"`
}

func Groups(observations []domain.Observation) []Group {
	byKey := map[string]*Group{}
	for _, obs := range observations {
		key := obs.RepoID + "\x00" + string(obs.Locus) + "\x00" + obs.Scope
		group := byKey[key]
		if group == nil {
			group = &Group{RepoID: obs.RepoID, Locus: obs.Locus, Scope: domain.ScopeLabel(obs.Scope)}
			byKey[key] = group
		}
		group.Observations = append(group.Observations, obs.Clone())
	}
	out := make([]Group, 0, len(byKey))
	for _, group := range byKey {
		domain.SortObservations(group.Observations)
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RepoID != out[j].RepoID {
			return out[i].RepoID < out[j].RepoID
		}
		if out[i].Locus != out[j].Locus {
			return out[i].Locus < out[j].Locus
		}
		return out[i].Scope < out[j].Scope
	})
	return out
}

func RenderList(observations []domain.Observation) string {
	if len(observations) == 0 {
		return "no active papercuts\n"
	}
	var buf bytes.Buffer
	for _, obs := range observations {
		fmt.Fprintf(&buf, "%s [%s] repo=%s locus=%s scope=%s state=%s\n", obs.ID, obs.Severity, obs.RepoID, obs.Locus, domain.ScopeLabel(obs.Scope), obs.State)
		fmt.Fprintf(&buf, "  expected: %s\n", oneLine(obs.Expected))
		fmt.Fprintf(&buf, "  observed: %s\n", oneLine(obs.Observed))
		fmt.Fprintf(&buf, "  impact: %s\n", oneLine(obs.Impact))
	}
	return buf.String()
}

func RenderReview(groups []Group) string {
	if len(groups) == 0 {
		return "no active papercuts\n"
	}
	var buf bytes.Buffer
	for _, group := range groups {
		fmt.Fprintf(&buf, "Group repo=%s locus=%s scope=%s\n", group.RepoID, group.Locus, group.Scope)
		for _, obs := range group.Observations {
			fmt.Fprintf(&buf, "- %s [%s]\n", obs.ID, obs.Severity)
			fmt.Fprintf(&buf, "  Expected: %s\n", oneLine(obs.Expected))
			fmt.Fprintf(&buf, "  Observed: %s\n", oneLine(obs.Observed))
			fmt.Fprintf(&buf, "  Impact: %s\n", oneLine(obs.Impact))
			if len(obs.Suggestions) > 0 {
				fmt.Fprintf(&buf, "  Suggestions:\n")
				for _, suggestion := range obs.Suggestions {
					fmt.Fprintf(&buf, "  - %s\n", oneLine(suggestion.Text))
				}
			}
		}
	}
	return buf.String()
}

func RenderJSON(value any) (string, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(body) + "\n", nil
}

func InstructionsAgents() string {
	return `## papercut

Use ` + "`papercut`" + ` to record local operational friction instead of burying it in chat.

- Add an observation with ` + "`papercut add --expected ... --observed ... --impact ... --locus repo`" + `.
- For sensitive details, prefer ` + "`papercut add --input-json -`" + ` so values do not enter shell history.
- Review active work blockers with ` + "`papercut review`" + `.
- Mark lifecycle progress with ` + "`papercut claim-fixed <id>`" + `, ` + "`papercut verify-fixed <id>`" + `, or ` + "`papercut dispose <id> --reason ...`" + `.
- Do not edit ` + "`PAPERCLIP.md`" + ` event blocks by hand.
`
}

func oneLine(value string) string {
	runes := []rune(value)
	for i, r := range runes {
		if r == '\n' || r == '\r' || r == '\t' {
			runes[i] = ' '
		}
	}
	return string(runes)
}
