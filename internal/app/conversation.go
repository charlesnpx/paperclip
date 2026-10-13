package app

import "github.com/charlesnpx/paperclip/internal/domain"

func (application *App) commitObservation(conversation domain.Conversation, plan func(domain.Snapshot) ([]domain.Event, domain.CommitResult, error)) (domain.CommitResult, error) {
	return application.store.Commit(application.lockTimeout, func(snapshot domain.Snapshot) ([]domain.Event, domain.CommitResult, error) {
		events, result, err := plan(snapshot)
		if err != nil || conversation == (domain.Conversation{}) {
			return events, result, err
		}
		observation, exists := snapshot.Observation(result.ObservationID)
		if exists && observation.HasConversation(conversation) {
			return events, result, nil
		}
		repoID := observation.RepoID
		if !exists {
			repoID = events[0].Context.RepoID
		}
		eventID, err := application.newID("evt")
		if err != nil {
			return nil, domain.CommitResult{}, err
		}
		event, err := domain.NewEvent(eventID, application.clock(), domain.EventConversationLinked, result.ObservationID, "", domain.Context{RepoID: repoID}, conversation)
		if err != nil {
			return nil, domain.CommitResult{}, err
		}
		return append(events, event), result, nil
	})
}
