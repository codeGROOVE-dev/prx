package prx

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// fetchPullRequestCompleteViaGraphQL fetches all PR data in a single GraphQL query.
func (c *Client) fetchPullRequestCompleteViaGraphQL(ctx context.Context, owner, repo string, prNumber int) (*PullRequestData, error) {
	data, err := c.executeGraphQL(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	pr := c.convertGraphQLToPullRequest(ctx, data, owner, repo)
	events := c.convertGraphQLToEventsComplete(ctx, data, owner, repo)
	requiredChecks := c.extractRequiredChecksFromGraphQL(data)

	events = filterEvents(events)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	upgradeWriteAccess(events)

	testState := c.calculateTestStateFromGraphQL(data)
	finalizePullRequest(&pr, events, requiredChecks, testState)

	return &PullRequestData{
		PullRequest: pr,
		Events:      events,
	}, nil
}

// executeGraphQL executes the GraphQL query and handles errors.
func (c *Client) executeGraphQL(ctx context.Context, owner, repo string, prNumber int) (*graphQLPullRequestComplete, error) {
	variables := map[string]any{
		"owner":  owner,
		"repo":   repo,
		"number": prNumber,
	}

	var result graphQLCompleteResponse
	if err := c.github.GraphQL(ctx, completeGraphQLQuery, variables, &result); err != nil {
		return nil, err
	}

	if len(result.Errors) > 0 {
		var errMsgs []string
		var hasPermissionError bool
		for _, e := range result.Errors {
			errMsgs = append(errMsgs, e.Message)
			msg := strings.ToLower(e.Message)
			if strings.Contains(msg, "not accessible by integration") ||
				strings.Contains(msg, "resource not accessible") ||
				strings.Contains(msg, "forbidden") ||
				strings.Contains(msg, "insufficient permissions") ||
				strings.Contains(msg, "requires authentication") {
				hasPermissionError = true
			}
		}

		errStr := strings.Join(errMsgs, "; ")
		if result.Data.Repository.PullRequest.Number == 0 {
			if hasPermissionError {
				return nil, fmt.Errorf(
					"fetching PR %s/%s#%d via GraphQL failed due to insufficient permissions: %s "+
						"(note: some fields like branchProtectionRule or refUpdateRule may require push access "+
						"even on public repositories; check token scopes or try using a token with 'repo' or 'public_repo' scope)",
					owner, repo, prNumber, errStr)
			}
			return nil, fmt.Errorf("fetching PR %s/%s#%d via GraphQL: %s", owner, repo, prNumber, errStr)
		}

		if hasPermissionError {
			c.logger.WarnContext(ctx, "GraphQL query returned permission errors but PR data was retrieved - some fields may be missing",
				"owner", owner,
				"repo", repo,
				"pr", prNumber,
				"errors", errStr,
				"note", "fields like branchProtectionRule or refUpdateRule require push access")
		} else {
			c.logger.WarnContext(ctx, "GraphQL query returned errors but PR data was retrieved",
				"owner", owner,
				"repo", repo,
				"pr", prNumber,
				"errors", errStr)
		}
	}

	return &result.Data.Repository.PullRequest, nil
}

// convertGraphQLToPullRequest converts GraphQL data to PullRequest.
func (c *Client) convertGraphQLToPullRequest(ctx context.Context, data *graphQLPullRequestComplete, owner, repo string) PullRequest {
	pr := PullRequest{
		Number:       data.Number,
		Title:        data.Title,
		Body:         truncate(data.Body),
		Author:       data.Author.Login,
		State:        strings.ToLower(data.State),
		CreatedAt:    data.CreatedAt,
		UpdatedAt:    data.UpdatedAt,
		Draft:        data.IsDraft,
		Additions:    data.Additions,
		Deletions:    data.Deletions,
		ChangedFiles: data.ChangedFiles,
		HeadSHA:      data.HeadRef.Target.OID,
	}

	if data.ClosedAt != nil {
		pr.ClosedAt = data.ClosedAt
	}
	if data.MergedAt != nil {
		pr.MergedAt = data.MergedAt
		pr.Merged = true
	}
	if data.MergedBy != nil {
		pr.MergedBy = data.MergedBy.Login
	}

	switch data.MergeStateStatus {
	case "CLEAN":
		pr.MergeableState = "clean"
	case "UNSTABLE":
		pr.MergeableState = "unstable"
	case "BLOCKED":
		pr.MergeableState = "blocked"
	case "BEHIND":
		pr.MergeableState = "behind"
	case "DIRTY":
		pr.MergeableState = "dirty"
	default:
		pr.MergeableState = strings.ToLower(data.MergeStateStatus)
	}

	if data.Author.Login != "" {
		pr.AuthorWriteAccess = c.writeAccessFromAssociation(ctx, owner, repo, data.Author.Login, data.AuthorAssociation)
		pr.AuthorBot = isBot(data.Author)
	}

	pr.Assignees = make([]string, 0)
	for _, assignee := range data.Assignees.Nodes {
		pr.Assignees = append(pr.Assignees, assignee.Login)
	}

	for _, label := range data.Labels.Nodes {
		pr.Labels = append(pr.Labels, label.Name)
	}

	for _, node := range data.Commits.Nodes {
		pr.Commits = append(pr.Commits, node.Commit.OID)
	}

	pr.Reviewers = buildReviewersMap(data)

	return pr
}

// buildReviewersMap constructs a map of reviewer login to their review state.
func buildReviewersMap(data *graphQLPullRequestComplete) map[string]ReviewState {
	reviewers := make(map[string]ReviewState)

	for _, request := range data.ReviewRequests.Nodes {
		reviewer := request.RequestedReviewer
		if reviewer.Login != "" {
			reviewers[reviewer.Login] = ReviewStatePending
		} else if reviewer.Name != "" {
			reviewers[reviewer.Name] = ReviewStatePending
		}
	}

	for i := range data.Reviews.Nodes {
		review := &data.Reviews.Nodes[i]
		if review.Author.Login == "" {
			continue
		}

		var state ReviewState
		switch strings.ToUpper(review.State) {
		case "APPROVED":
			state = ReviewStateApproved
		case "CHANGES_REQUESTED":
			state = ReviewStateChangesRequested
		case "COMMENTED":
			state = ReviewStateCommented
		default:
			continue
		}

		reviewers[review.Author.Login] = state
	}

	return reviewers
}

// convertGraphQLToEventsComplete converts GraphQL data to Events.
func (c *Client) convertGraphQLToEventsComplete(ctx context.Context, data *graphQLPullRequestComplete, owner, repo string) []Event {
	var events []Event

	events = append(events, Event{
		Kind:        "pr_opened",
		Timestamp:   data.CreatedAt,
		Actor:       data.Author.Login,
		Body:        truncate(data.Body),
		Bot:         isBot(data.Author),
		WriteAccess: c.writeAccessFromAssociation(ctx, owner, repo, data.Author.Login, data.AuthorAssociation),
	})

	for _, node := range data.Commits.Nodes {
		event := Event{
			Kind:        EventKindCommit,
			Timestamp:   node.Commit.CommittedDate,
			Body:        node.Commit.OID,
			Description: truncate(node.Commit.Message),
		}
		if node.Commit.Author.User != nil {
			event.Actor = node.Commit.Author.User.Login
			event.Bot = isBot(*node.Commit.Author.User)
		} else {
			event.Actor = node.Commit.Author.Name
		}
		events = append(events, event)
	}

	for i := range data.Reviews.Nodes {
		review := &data.Reviews.Nodes[i]
		if review.State == "" {
			continue
		}
		timestamp := review.CreatedAt
		if review.SubmittedAt != nil {
			timestamp = *review.SubmittedAt
		}
		event := Event{
			Kind:        EventKindReview,
			Timestamp:   timestamp,
			Actor:       review.Author.Login,
			Body:        truncate(review.Body),
			Outcome:     strings.ToLower(review.State),
			Question:    containsQuestion(review.Body),
			Bot:         isBot(review.Author),
			WriteAccess: c.writeAccessFromAssociation(ctx, owner, repo, review.Author.Login, review.AuthorAssociation),
		}
		events = append(events, event)
	}

	for i := range data.ReviewThreads.Nodes {
		thread := &data.ReviewThreads.Nodes[i]
		for j := range thread.Comments.Nodes {
			comment := &thread.Comments.Nodes[j]
			event := Event{
				Kind:        EventKindReviewComment,
				Timestamp:   comment.CreatedAt,
				Actor:       comment.Author.Login,
				Body:        truncate(comment.Body),
				Question:    containsQuestion(comment.Body),
				Bot:         isBot(comment.Author),
				WriteAccess: c.writeAccessFromAssociation(ctx, owner, repo, comment.Author.Login, comment.AuthorAssociation),
				Outdated:    comment.Outdated,
			}
			events = append(events, event)
		}
	}

	for _, comment := range data.Comments.Nodes {
		event := Event{
			Kind:        EventKindComment,
			Timestamp:   comment.CreatedAt,
			Actor:       comment.Author.Login,
			Body:        truncate(comment.Body),
			Question:    containsQuestion(comment.Body),
			Bot:         isBot(comment.Author),
			WriteAccess: c.writeAccessFromAssociation(ctx, owner, repo, comment.Author.Login, comment.AuthorAssociation),
		}
		events = append(events, event)
	}

	if data.HeadRef.Target.StatusCheckRollup != nil {
		for i := range data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes {
			node := &data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes[i]
			switch node.TypeName {
			case "CheckRun":
				var description string
				switch {
				case node.Title != "" && node.Summary != "":
					description = fmt.Sprintf("%s: %s", node.Title, node.Summary)
				case node.Title != "":
					description = node.Title
				case node.Summary != "":
					description = node.Summary
				default:
					// No description available
				}

				if node.StartedAt != nil {
					events = append(events, Event{
						Kind:        EventKindCheckRun,
						Timestamp:   *node.StartedAt,
						Body:        node.Name,
						Outcome:     strings.ToLower(node.Status),
						Bot:         true,
						Description: description,
					})
				}

				if node.CompletedAt != nil {
					events = append(events, Event{
						Kind:        EventKindCheckRun,
						Timestamp:   *node.CompletedAt,
						Body:        node.Name,
						Outcome:     strings.ToLower(node.Conclusion),
						Bot:         true,
						Description: description,
					})
				}

			case "StatusContext":
				if node.CreatedAt == nil {
					continue
				}
				event := Event{
					Kind:        EventKindStatusCheck,
					Timestamp:   *node.CreatedAt,
					Outcome:     strings.ToLower(node.State),
					Body:        node.Context,
					Description: node.Description,
				}
				if node.Creator != nil {
					event.Actor = node.Creator.Login
					event.Bot = isBot(*node.Creator)
				}
				events = append(events, event)

			default:
				// Unknown check type, skip
			}
		}
	}

	for _, item := range data.TimelineItems.Nodes {
		event := c.parseGraphQLTimelineEvent(ctx, item, owner, repo)
		if event != nil {
			events = append(events, *event)
		}
	}

	if data.ClosedAt != nil && !data.IsDraft {
		event := Event{
			Kind:      "pr_closed",
			Timestamp: *data.ClosedAt,
		}
		if data.MergedBy != nil {
			event.Actor = data.MergedBy.Login
			event.Kind = EventKindPRMerged
			event.Bot = isBot(*data.MergedBy)
		}
		events = append(events, event)
	}

	return events
}

// parseGraphQLTimelineEvent parses a single timeline event.
//
//nolint:gocognit,maintidx,revive // High complexity justified - must handle all GitHub timeline event types
func (*Client) parseGraphQLTimelineEvent(_ context.Context, item map[string]any, _, _ string) *Event {
	typename, ok := item["__typename"].(string)
	if !ok {
		return nil
	}

	getTime := func(key string) *time.Time {
		if str, ok := item[key].(string); ok {
			if t, err := time.Parse(time.RFC3339, str); err == nil {
				return &t
			}
		}
		return nil
	}

	getActor := func() string {
		if actor, ok := item["actor"].(map[string]any); ok {
			if login, ok := actor["login"].(string); ok {
				return login
			}
		}
		return "unknown"
	}

	isActorBot := func() bool {
		if actor, ok := item["actor"].(map[string]any); ok {
			var actorObj graphQLActor
			if login, ok := actor["login"].(string); ok {
				actorObj.Login = login
			}
			if id, ok := actor["id"].(string); ok {
				actorObj.ID = id
			}
			if typ, ok := actor["__typename"].(string); ok {
				actorObj.Type = typ
			}
			return isBot(actorObj)
		}
		return false
	}

	createdAt := getTime("createdAt")
	if createdAt == nil {
		return nil
	}

	event := &Event{
		Timestamp: *createdAt,
		Actor:     getActor(),
		Bot:       isActorBot(),
	}

	switch typename {
	case "AssignedEvent":
		event.Kind = EventKindAssigned
		if assignee, ok := item["assignee"].(map[string]any); ok {
			if login, ok := assignee["login"].(string); ok {
				event.Target = login
			}
		}

	case "UnassignedEvent":
		event.Kind = EventKindUnassigned
		if assignee, ok := item["assignee"].(map[string]any); ok {
			if login, ok := assignee["login"].(string); ok {
				event.Target = login
			}
		}

	case "LabeledEvent":
		event.Kind = EventKindLabeled
		if label, ok := item["label"].(map[string]any); ok {
			if name, ok := label["name"].(string); ok {
				event.Target = name
			}
		}

	case "UnlabeledEvent":
		event.Kind = EventKindUnlabeled
		if label, ok := item["label"].(map[string]any); ok {
			if name, ok := label["name"].(string); ok {
				event.Target = name
			}
		}

	case "MilestonedEvent":
		event.Kind = EventKindMilestoned
		if title, ok := item["milestoneTitle"].(string); ok {
			event.Target = title
		}

	case "DemilestonedEvent":
		event.Kind = EventKindDemilestoned
		if title, ok := item["milestoneTitle"].(string); ok {
			event.Target = title
		}

	case "ReviewRequestedEvent":
		event.Kind = EventKindReviewRequested
		if reviewer, ok := item["requestedReviewer"].(map[string]any); ok {
			if login, ok := reviewer["login"].(string); ok {
				event.Target = login
			} else if name, ok := reviewer["name"].(string); ok {
				event.Target = name
			}
		}

	case "ReviewRequestRemovedEvent":
		event.Kind = EventKindReviewRequestRemoved
		if reviewer, ok := item["requestedReviewer"].(map[string]any); ok {
			if login, ok := reviewer["login"].(string); ok {
				event.Target = login
			} else if name, ok := reviewer["name"].(string); ok {
				event.Target = name
			}
		}

	case "MentionedEvent":
		event.Kind = EventKindMentioned
		event.Body = "User was mentioned"

	case "ReadyForReviewEvent":
		event.Kind = EventKindReadyForReview

	case "ConvertToDraftEvent":
		event.Kind = EventKindConvertToDraft

	case "ClosedEvent":
		event.Kind = EventKindClosed

	case "ReopenedEvent":
		event.Kind = EventKindReopened

	case "MergedEvent":
		event.Kind = "merged"

	case "AutoMergeEnabledEvent":
		event.Kind = EventKindAutoMergeEnabled

	case "AutoMergeDisabledEvent":
		event.Kind = EventKindAutoMergeDisabled

	case "ReviewDismissedEvent":
		event.Kind = EventKindReviewDismissed
		if msg, ok := item["dismissalMessage"].(string); ok {
			event.Body = msg
		}

	case "BaseRefChangedEvent":
		event.Kind = EventKindBaseRefChanged

	case "BaseRefForcePushedEvent":
		event.Kind = EventKindBaseRefForcePushed

	case "HeadRefForcePushedEvent":
		event.Kind = EventKindHeadRefForcePushed

	case "HeadRefDeletedEvent":
		event.Kind = EventKindHeadRefDeleted

	case "HeadRefRestoredEvent":
		event.Kind = EventKindHeadRefRestored

	case "RenamedTitleEvent":
		event.Kind = "renamed_title"
		if prev, ok := item["previousTitle"].(string); ok {
			if curr, ok := item["currentTitle"].(string); ok {
				event.Body = fmt.Sprintf("Renamed from %q to %q", prev, curr)
			}
		}

	case "LockedEvent":
		event.Kind = EventKindLocked

	case "UnlockedEvent":
		event.Kind = EventKindUnlocked

	case "AddedToMergeQueueEvent":
		event.Kind = "added_to_merge_queue"

	case "RemovedFromMergeQueueEvent":
		event.Kind = "removed_from_merge_queue"

	case "AutomaticBaseChangeSucceededEvent":
		event.Kind = "automatic_base_change_succeeded"

	case "AutomaticBaseChangeFailedEvent":
		event.Kind = "automatic_base_change_failed"

	case "ConnectedEvent":
		event.Kind = EventKindConnected

	case "DisconnectedEvent":
		event.Kind = EventKindDisconnected

	case "CrossReferencedEvent":
		event.Kind = "cross_referenced"

	case "ReferencedEvent":
		event.Kind = EventKindReferenced

	case "SubscribedEvent":
		event.Kind = EventKindSubscribed

	case "UnsubscribedEvent":
		event.Kind = EventKindUnsubscribed

	case "DeployedEvent":
		event.Kind = "deployed"

	case "DeploymentEnvironmentChangedEvent":
		event.Kind = EventKindDeploymentEnvironmentChanged

	case "PinnedEvent":
		event.Kind = EventKindPinned

	case "UnpinnedEvent":
		event.Kind = EventKindUnpinned

	case "TransferredEvent":
		event.Kind = EventKindTransferred

	case "UserBlockedEvent":
		event.Kind = "user_blocked"

	default:
		return nil
	}

	return event
}

// writeAccessFromAssociation calculates write access from association.
func (c *Client) writeAccessFromAssociation(ctx context.Context, owner, repo, user, association string) int {
	if user == "" {
		return WriteAccessNA
	}

	switch association {
	case "OWNER", "COLLABORATOR":
		return WriteAccessDefinitely
	case "MEMBER":
		return c.checkCollaboratorPermission(ctx, owner, repo, user)
	case "CONTRIBUTOR", "NONE", "FIRST_TIME_CONTRIBUTOR", "FIRST_TIMER":
		return WriteAccessUnlikely
	default:
		return WriteAccessNA
	}
}

// checkCollaboratorPermission checks if a user has write access.
func (c *Client) checkCollaboratorPermission(ctx context.Context, owner, repo, user string) int {
	cacheKey := collaboratorsCacheKey(owner, repo)

	collabs, err := c.collaboratorsCache.GetSet(cacheKey, func() (map[string]string, error) {
		result, fetchErr := c.github.Collaborators(ctx, owner, repo)
		if fetchErr != nil {
			c.logger.WarnContext(ctx, "failed to fetch collaborators for write access check",
				"owner", owner,
				"repo", repo,
				"user", user,
				"error", fetchErr)

			// On any error (including 403 Forbidden), return the error
			// so that checkCollaboratorPermission returns WriteAccessLikely
			return nil, fetchErr
		}

		return result, nil
	})
	if err != nil {
		return WriteAccessLikely
	}

	switch collabs[user] {
	case "admin", "maintain", "write":
		return WriteAccessDefinitely
	case "read", "triage", "none":
		return WriteAccessNo
	default:
		return WriteAccessUnlikely
	}
}

// extractRequiredChecksFromGraphQL gets required checks from GraphQL response.
func (*Client) extractRequiredChecksFromGraphQL(data *graphQLPullRequestComplete) []string {
	checkMap := make(map[string]bool)

	if data.BaseRef.RefUpdateRule != nil {
		for _, check := range data.BaseRef.RefUpdateRule.RequiredStatusCheckContexts {
			checkMap[check] = true
		}
	}

	if data.BaseRef.BranchProtectionRule != nil {
		for _, check := range data.BaseRef.BranchProtectionRule.RequiredStatusCheckContexts {
			checkMap[check] = true
		}
	}

	var checks []string
	for check := range checkMap {
		checks = append(checks, check)
	}
	return checks
}

// calculateTestStateFromGraphQL determines test state from check runs.
func (*Client) calculateTestStateFromGraphQL(data *graphQLPullRequestComplete) string {
	if data.HeadRef.Target.StatusCheckRollup == nil {
		return ""
	}

	var hasFailure, hasRunning, hasQueued bool

	for i := range data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes {
		node := &data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes[i]
		if node.TypeName != "CheckRun" {
			continue
		}

		if !strings.Contains(strings.ToLower(node.Name), "test") &&
			!strings.Contains(strings.ToLower(node.Name), "check") &&
			!strings.Contains(strings.ToLower(node.Name), "ci") {
			continue
		}

		switch strings.ToLower(node.Status) {
		case "queued":
			hasQueued = true
		case "in_progress":
			hasRunning = true
		default:
			// Other statuses don't affect state
		}

		switch strings.ToLower(node.Conclusion) {
		case "failure", "timed_out", "action_required":
			hasFailure = true
		default:
			// Other conclusions don't affect state
		}
	}

	if hasFailure {
		return "failing"
	}
	if hasRunning {
		return "running"
	}
	if hasQueued {
		return "queued"
	}
	return "passing"
}
