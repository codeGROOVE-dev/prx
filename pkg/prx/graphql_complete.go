package prx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// completeGraphQLQuery is the complete GraphQL query that fetches ALL data
// currently fetched by multiple REST API calls, preserving all fields.
const completeGraphQLQuery = `
query($owner: String!, $repo: String!, $number: Int!, $prCursor: String, $reviewCursor: String, $timelineCursor: String, $commentCursor: String) {
	repository(owner: $owner, name: $repo) {
		pullRequest(number: $number) {
			id
			number
			title
			body
			state
			createdAt
			updatedAt
			closedAt
			mergedAt
			isDraft
			additions
			deletions
			changedFiles
			mergeable
			mergeStateStatus
			authorAssociation

			author {
				login
				... on User {
					id
				}
				... on Bot {
					id
				}
			}

			mergedBy {
				login
				... on User {
					id
				}
			}

			assignees(first: 100) {
				nodes {
					login
					... on User {
						id
					}
				}
			}

			labels(first: 100) {
				nodes {
					name
				}
			}

			participants(first: 100) {
				nodes {
					login
					... on User {
						id
					}
				}
			}

			baseRef {
				name
				target {
					... on Commit {
						oid
					}
				}
				refUpdateRule {
					requiredStatusCheckContexts
				}
				branchProtectionRule {
					requiredStatusCheckContexts
					requiresStatusChecks
					requiredApprovingReviewCount
					requiresApprovingReviews
				}
			}

			headRef {
				name
				target {
					... on Commit {
						oid
						statusCheckRollup {
							state
							contexts(first: 100) {
								nodes {
									__typename
									... on CheckRun {
										name
										status
										conclusion
										startedAt
										completedAt
										detailsUrl
										title: title
										text: text
										summary: summary
										databaseId
									}
									... on StatusContext {
										context
										state
										description
										targetUrl
										createdAt
										creator {
											login
											... on Bot {
												id
											}
										}
									}
								}
							}
						}
					}
				}
			}

			commits(first: 100, after: $prCursor) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					commit {
						oid
						message
						committedDate
						author {
							name
							email
							user {
								login
								... on User {
									id
								}
							}
						}
					}
				}
			}

			reviews(first: 100, after: $reviewCursor) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					id
					state
					body
					createdAt
					submittedAt
					authorAssociation
					author {
						login
						... on User {
							id
						}
						... on Bot {
							id
						}
					}
				}
			}

			reviewThreads(first: 100) {
				nodes {
					isResolved
					isOutdated
					comments(first: 100) {
						nodes {
							id
							body
							createdAt
							authorAssociation
							author {
								login
								... on User {
									id
								}
								... on Bot {
									id
								}
							}
						}
					}
				}
			}

			comments(first: 100, after: $commentCursor) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					id
					body
					createdAt
					authorAssociation
					author {
						login
						... on User {
							id
						}
						... on Bot {
							id
						}
					}
				}
			}

			timelineItems(first: 100, after: $timelineCursor) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					__typename
					... on AssignedEvent {
						id
						createdAt
						actor {
							login
							... on User {
								id
							}
						}
						assignee {
							... on User {
								login
								id
							}
							... on Bot {
								login
								id
							}
						}
					}
					... on UnassignedEvent {
						id
						createdAt
						actor {
							login
						}
						assignee {
							... on User {
								login
								id
							}
						}
					}
					... on LabeledEvent {
						id
						createdAt
						label {
							name
						}
						actor {
							login
						}
					}
					... on UnlabeledEvent {
						id
						createdAt
						label {
							name
						}
						actor {
							login
						}
					}
					... on MilestonedEvent {
						id
						createdAt
						milestoneTitle
						actor {
							login
						}
					}
					... on DemilestonedEvent {
						id
						createdAt
						milestoneTitle
						actor {
							login
						}
					}
					... on ReviewRequestedEvent {
						id
						createdAt
						actor {
							login
						}
						requestedReviewer {
							... on User {
								login
								id
							}
							... on Team {
								name
								id
							}
							... on Bot {
								login
								id
							}
						}
					}
					... on ReviewRequestRemovedEvent {
						id
						createdAt
						actor {
							login
						}
						requestedReviewer {
							... on User {
								login
							}
							... on Team {
								name
							}
						}
					}
					... on ClosedEvent {
						id
						createdAt
						actor {
							login
						}
					}
					... on ReopenedEvent {
						id
						createdAt
						actor {
							login
						}
					}
					... on MergedEvent {
						id
						createdAt
						actor {
							login
						}
					}
					... on MentionedEvent {
						id
						createdAt
						actor {
							login
						}
					}
					... on ReadyForReviewEvent {
						id
						createdAt
						actor {
							login
						}
					}
					... on ConvertToDraftEvent {
						id
						createdAt
						actor {
							login
						}
					}
				}
			}
		}
	}

	rateLimit {
		cost
		remaining
		resetAt
		limit
	}
}`

// fetchPullRequestCompleteViaGraphQL fetches ALL pull request data in a single GraphQL query.
// This replaces 13+ REST API calls with a single comprehensive query.
func (c *Client) fetchPullRequestCompleteViaGraphQL(ctx context.Context, owner, repo string, prNumber int) (*PullRequestData, error) {
	gc, ok := c.github.(*githubClient)
	if !ok {
		return nil, fmt.Errorf("cannot access GitHub client for GraphQL")
	}

	// Execute the query (may need pagination for large PRs)
	data, err := c.executePaginatedGraphQL(ctx, gc, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	// Convert to our internal format
	pr := c.convertGraphQLToPullRequest(ctx, data, owner, repo)
	events := c.convertGraphQLToEventsComplete(ctx, data, owner, repo)

	// Calculate required checks
	requiredChecks := c.extractRequiredChecksFromGraphQL(data)

	// Process events (upgrade write access, etc.)
	events = processEvents(events)

	// Calculate test state
	testState := c.calculateTestStateFromGraphQL(data)

	// Finalize PR data
	finalizePullRequest(&pr, events, requiredChecks, testState)

	return &PullRequestData{
		PullRequest: pr,
		Events:      events,
	}, nil
}

// executePaginatedGraphQL handles pagination for large PRs
func (c *Client) executePaginatedGraphQL(ctx context.Context, gc *githubClient, owner, repo string, prNumber int) (*graphQLPullRequestComplete, error) {
	variables := map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"number": prNumber,
	}

	requestBody := map[string]interface{}{
		"query":     completeGraphQLQuery,
		"variables": variables,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gc.api+"/graphql", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating GraphQL request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", gc.token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v4+json")
	// Enable beta features for authorAssociation
	req.Header.Set("Accept", "application/vnd.github.stone-age-preview+json")

	resp, err := gc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing GraphQL request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		limitedBody := io.LimitReader(resp.Body, 1024*1024)
		body, err := io.ReadAll(limitedBody)
		if err != nil {
			return nil, fmt.Errorf("GraphQL request failed with status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("GraphQL request failed with status %d: %s", resp.StatusCode, body)
	}

	var result graphQLCompleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding GraphQL response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL errors: %v", result.Errors)
	}

	c.logger.InfoContext(ctx, "GraphQL query completed",
		"cost", result.Data.RateLimit.Cost,
		"remaining", result.Data.RateLimit.Remaining,
		"limit", result.Data.RateLimit.Limit)

	// TODO: Handle pagination if needed for commits, reviews, timeline, comments
	// For now, we fetch first 100 of each which should cover most PRs

	return &result.Data.Repository.PullRequest, nil
}

// graphQLCompleteResponse represents the complete GraphQL response
type graphQLCompleteResponse struct {
	Data struct {
		Repository struct {
			PullRequest graphQLPullRequestComplete `json:"pullRequest"`
		} `json:"repository"`
		RateLimit struct {
			Cost      int       `json:"cost"`
			Remaining int       `json:"remaining"`
			ResetAt   time.Time `json:"resetAt"`
			Limit     int       `json:"limit"`
		} `json:"rateLimit"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// graphQLPullRequestComplete includes ALL fields we need
type graphQLPullRequestComplete struct {
	ID                string     `json:"id"`
	Number            int        `json:"number"`
	Title             string     `json:"title"`
	Body              string     `json:"body"`
	State             string     `json:"state"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	ClosedAt          *time.Time `json:"closedAt"`
	MergedAt          *time.Time `json:"mergedAt"`
	IsDraft           bool       `json:"isDraft"`
	Additions         int        `json:"additions"`
	Deletions         int        `json:"deletions"`
	ChangedFiles      int        `json:"changedFiles"`
	Mergeable         string     `json:"mergeable"`
	MergeStateStatus  string     `json:"mergeStateStatus"`
	AuthorAssociation string     `json:"authorAssociation"`

	Author graphQLActor `json:"author"`
	MergedBy *graphQLActor `json:"mergedBy"`

	Assignees struct {
		Nodes []graphQLActor `json:"nodes"`
	} `json:"assignees"`

	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`

	BaseRef struct {
		Name   string `json:"name"`
		Target struct {
			OID string `json:"oid"`
		} `json:"target"`
		RefUpdateRule *struct {
			RequiredStatusCheckContexts []string `json:"requiredStatusCheckContexts"`
		} `json:"refUpdateRule"`
		BranchProtectionRule *struct {
			RequiredStatusCheckContexts  []string `json:"requiredStatusCheckContexts"`
			RequiresStatusChecks         bool     `json:"requiresStatusChecks"`
			RequiredApprovingReviewCount int      `json:"requiredApprovingReviewCount"`
		} `json:"branchProtectionRule"`
	} `json:"baseRef"`

	HeadRef struct {
		Name   string `json:"name"`
		Target struct {
			OID              string `json:"oid"`
			StatusCheckRollup *struct {
				State    string `json:"state"`
				Contexts struct {
					Nodes []graphQLStatusCheckNode `json:"nodes"`
				} `json:"contexts"`
			} `json:"statusCheckRollup"`
		} `json:"target"`
	} `json:"headRef"`

	Commits struct {
		PageInfo graphQLPageInfo `json:"pageInfo"`
		Nodes    []struct {
			Commit struct {
				OID           string    `json:"oid"`
				Message       string    `json:"message"`
				CommittedDate time.Time `json:"committedDate"`
				Author        struct {
					Name  string        `json:"name"`
					Email string        `json:"email"`
					User  *graphQLActor `json:"user"`
				} `json:"author"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`

	Reviews struct {
		PageInfo graphQLPageInfo `json:"pageInfo"`
		Nodes    []struct {
			ID                string        `json:"id"`
			State             string        `json:"state"`
			Body              string        `json:"body"`
			CreatedAt         time.Time     `json:"createdAt"`
			SubmittedAt       *time.Time    `json:"submittedAt"`
			AuthorAssociation string        `json:"authorAssociation"`
			Author            graphQLActor  `json:"author"`
		} `json:"nodes"`
	} `json:"reviews"`

	ReviewThreads struct {
		Nodes []struct {
			IsResolved bool `json:"isResolved"`
			IsOutdated bool `json:"isOutdated"`
			Comments   struct {
				Nodes []struct {
					ID                string       `json:"id"`
					Body              string       `json:"body"`
					CreatedAt         time.Time    `json:"createdAt"`
					AuthorAssociation string       `json:"authorAssociation"`
					Author            graphQLActor `json:"author"`
				} `json:"nodes"`
			} `json:"comments"`
		} `json:"nodes"`
	} `json:"reviewThreads"`

	Comments struct {
		PageInfo graphQLPageInfo `json:"pageInfo"`
		Nodes    []struct {
			ID                string       `json:"id"`
			Body              string       `json:"body"`
			CreatedAt         time.Time    `json:"createdAt"`
			AuthorAssociation string       `json:"authorAssociation"`
			Author            graphQLActor `json:"author"`
		} `json:"nodes"`
	} `json:"comments"`

	TimelineItems struct {
		PageInfo graphQLPageInfo          `json:"pageInfo"`
		Nodes    []map[string]interface{} `json:"nodes"`
	} `json:"timelineItems"`

}

// graphQLActor represents any GitHub actor (User, Bot, Organization)
type graphQLActor struct {
	Login string `json:"login"`
	ID    string `json:"id,omitempty"`
}

// isBotFromGraphQL determines if an actor is a bot
func isBotFromGraphQL(actor graphQLActor) bool {
	if actor.Login == "" {
		return false
	}
	// Check for bot patterns in login
	login := actor.Login
	if strings.HasSuffix(login, "[bot]") ||
	   strings.HasSuffix(login, "-bot") ||
	   strings.HasSuffix(login, "-robot") {
		return true
	}
	// In GraphQL, bots have different IDs than users
	// Bot IDs typically start with "BOT_" or have specific patterns
	// This is a heuristic that may need adjustment
	return strings.HasPrefix(actor.ID, "BOT_") || strings.Contains(actor.ID, "Bot")
}

// graphQLStatusCheckNode can be either CheckRun or StatusContext
type graphQLStatusCheckNode struct {
	TypeName    string     `json:"__typename"`
	Name        string     `json:"name,omitempty"`        // CheckRun
	Status      string     `json:"status,omitempty"`      // CheckRun
	Conclusion  string     `json:"conclusion,omitempty"`  // CheckRun
	StartedAt   *time.Time `json:"startedAt,omitempty"`   // CheckRun
	CompletedAt *time.Time `json:"completedAt,omitempty"` // CheckRun
	DetailsURL  string     `json:"detailsUrl,omitempty"`  // CheckRun
	Title       string     `json:"title,omitempty"`       // CheckRun
	Text        string     `json:"text,omitempty"`        // CheckRun
	Summary     string     `json:"summary,omitempty"`     // CheckRun
	DatabaseID  int        `json:"databaseId,omitempty"`  // CheckRun
	App         *struct {
		Name       string `json:"name"`
		DatabaseID int    `json:"databaseId"`
	} `json:"app,omitempty"` // CheckRun

	Context     string        `json:"context,omitempty"`     // StatusContext
	State       string        `json:"state,omitempty"`       // StatusContext
	Description string        `json:"description,omitempty"` // StatusContext
	TargetURL   string        `json:"targetUrl,omitempty"`   // StatusContext
	CreatedAt   *time.Time    `json:"createdAt,omitempty"`   // StatusContext
	Creator     *graphQLActor `json:"creator,omitempty"`     // StatusContext
}

// graphQLPageInfo for pagination
type graphQLPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// graphQLReviewerNode can be User, Team, or Bot
type graphQLReviewerNode struct {
	Login string `json:"login,omitempty"` // User/Bot
	Name  string `json:"name,omitempty"`  // Team
	ID    string `json:"id"`
}

// convertGraphQLToPullRequest converts GraphQL data to PullRequest
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

	// Handle nullable fields
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

	// Convert mergeable state
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

	// Author write access
	if data.Author.Login != "" {
		pr.AuthorWriteAccess = c.writeAccessFromAssociation(ctx, owner, repo, data.Author.Login, data.AuthorAssociation)
	}

	// Assignees
	for _, assignee := range data.Assignees.Nodes {
		pr.Assignees = append(pr.Assignees, assignee.Login)
	}

	// Labels
	for _, label := range data.Labels.Nodes {
		pr.Labels = append(pr.Labels, label.Name)
	}

	// Requested reviewers
	// RequestedReviewers field removed from GraphQL query
	// It's not critical data and can be omitted

	return pr
}

// convertGraphQLToEvents converts GraphQL data to Events
func (c *Client) convertGraphQLToEventsComplete(ctx context.Context, data *graphQLPullRequestComplete, owner, repo string) []Event {
	var events []Event

	// PR opened event
	events = append(events, Event{
		Kind:        "pr_opened",
		Timestamp:   data.CreatedAt,
		Actor:       data.Author.Login,
		Body:        truncate(data.Body),
		Bot:         isBotFromGraphQL(data.Author),
		WriteAccess: c.writeAccessFromAssociation(ctx, owner, repo, data.Author.Login, data.AuthorAssociation),
	})

	// Commits
	for _, node := range data.Commits.Nodes {
		event := Event{
			Kind:      "commit",
			Timestamp: node.Commit.CommittedDate,
			Body:      truncate(node.Commit.Message),
		}
		if node.Commit.Author.User != nil {
			event.Actor = node.Commit.Author.User.Login
			event.Bot = isBotFromGraphQL(*node.Commit.Author.User)
		} else {
			event.Actor = node.Commit.Author.Name
		}
		events = append(events, event)
	}

	// Reviews
	for _, review := range data.Reviews.Nodes {
		if review.State == "" {
			continue
		}
		timestamp := review.CreatedAt
		if review.SubmittedAt != nil {
			timestamp = *review.SubmittedAt
		}
		event := Event{
			Kind:        "review",
			Timestamp:   timestamp,
			Actor:       review.Author.Login,
			Body:        truncate(review.Body),
			Outcome:     strings.ToLower(review.State),
			Question:    containsQuestion(review.Body),
			Bot:         isBotFromGraphQL(review.Author),
			WriteAccess: c.writeAccessFromAssociation(ctx, owner, repo, review.Author.Login, review.AuthorAssociation),
		}
		events = append(events, event)
	}

	// Review comments
	for _, thread := range data.ReviewThreads.Nodes {
		for _, comment := range thread.Comments.Nodes {
			event := Event{
				Kind:        "review_comment",
				Timestamp:   comment.CreatedAt,
				Actor:       comment.Author.Login,
				Body:        truncate(comment.Body),
				Question:    containsQuestion(comment.Body),
				Bot:         isBotFromGraphQL(comment.Author),
				WriteAccess: c.writeAccessFromAssociation(ctx, owner, repo, comment.Author.Login, comment.AuthorAssociation),
			}
			events = append(events, event)
		}
	}

	// Issue comments
	for _, comment := range data.Comments.Nodes {
		event := Event{
			Kind:        "comment",
			Timestamp:   comment.CreatedAt,
			Actor:       comment.Author.Login,
			Body:        truncate(comment.Body),
			Question:    containsQuestion(comment.Body),
			Bot:         isBotFromGraphQL(comment.Author),
			WriteAccess: c.writeAccessFromAssociation(ctx, owner, repo, comment.Author.Login, comment.AuthorAssociation),
		}
		events = append(events, event)
	}

	// Status checks and check runs
	if data.HeadRef.Target.StatusCheckRollup != nil {
		for _, node := range data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes {
			switch node.TypeName {
			case "CheckRun":
				var timestamp time.Time
				var outcome string

				if node.CompletedAt != nil {
					timestamp = *node.CompletedAt
					outcome = strings.ToLower(node.Conclusion)
				} else if node.StartedAt != nil {
					timestamp = *node.StartedAt
					outcome = strings.ToLower(node.Status)
				} else {
					continue
				}

				event := Event{
					Kind:      "check_run",
					Timestamp: timestamp,
					Body:      node.Name,
					Outcome:   outcome,
					Bot:       true, // Check runs are always from apps
				}

				// Build description
				switch {
				case node.Title != "" && node.Summary != "":
					event.Description = fmt.Sprintf("%s: %s", node.Title, node.Summary)
				case node.Title != "":
					event.Description = node.Title
				case node.Summary != "":
					event.Description = node.Summary
				default:
					// No description available
				}

				events = append(events, event)

			case "StatusContext":
				if node.CreatedAt == nil {
					continue
				}
				event := Event{
					Kind:        "status_check",
					Timestamp:   *node.CreatedAt,
					Outcome:     strings.ToLower(node.State),
					Body:        node.Context,
					Description: node.Description,
				}
				if node.Creator != nil {
					event.Actor = node.Creator.Login
					event.Bot = isBotFromGraphQL(*node.Creator)
				}
				events = append(events, event)
			}
		}
	}

	// Timeline events
	for _, item := range data.TimelineItems.Nodes {
		event := c.parseGraphQLTimelineEvent(ctx, item, owner, repo)
		if event != nil {
			events = append(events, *event)
		}
	}

	// Add closed/merged events if applicable
	if data.ClosedAt != nil && !data.IsDraft {
		event := Event{
			Kind:      "pr_closed",
			Timestamp: *data.ClosedAt,
		}
		if data.MergedBy != nil {
			event.Actor = data.MergedBy.Login
			event.Kind = "pr_merged"
		}
		events = append(events, event)
	}

	return events
}

// parseGraphQLTimelineEvent parses a single timeline event
func (c *Client) parseGraphQLTimelineEvent(ctx context.Context, item map[string]interface{}, owner, repo string) *Event {
	typename, _ := item["__typename"].(string)

	// Helper to extract time
	getTime := func(key string) *time.Time {
		if str, ok := item[key].(string); ok {
			if t, err := time.Parse(time.RFC3339, str); err == nil {
				return &t
			}
		}
		return nil
	}

	// Helper to extract actor
	getActor := func() string {
		if actor, ok := item["actor"].(map[string]interface{}); ok {
			if login, ok := actor["login"].(string); ok {
				return login
			}
		}
		return "unknown"
	}

	createdAt := getTime("createdAt")
	if createdAt == nil {
		return nil
	}

	event := &Event{
		Timestamp: *createdAt,
		Actor:     getActor(),
	}

	switch typename {
	case "AssignedEvent":
		event.Kind = "assigned"
		if assignee, ok := item["assignee"].(map[string]interface{}); ok {
			event.Target, _ = assignee["login"].(string)
		}

	case "UnassignedEvent":
		event.Kind = "unassigned"
		if assignee, ok := item["assignee"].(map[string]interface{}); ok {
			event.Target, _ = assignee["login"].(string)
		}

	case "LabeledEvent":
		event.Kind = "labeled"
		if label, ok := item["label"].(map[string]interface{}); ok {
			event.Target, _ = label["name"].(string)
		}

	case "UnlabeledEvent":
		event.Kind = "unlabeled"
		if label, ok := item["label"].(map[string]interface{}); ok {
			event.Target, _ = label["name"].(string)
		}

	case "MilestonedEvent":
		event.Kind = "milestoned"
		event.Target, _ = item["milestoneTitle"].(string)

	case "DemilestonedEvent":
		event.Kind = "demilestoned"
		event.Target, _ = item["milestoneTitle"].(string)

	case "ReviewRequestedEvent":
		event.Kind = "review_requested"
		if reviewer, ok := item["requestedReviewer"].(map[string]interface{}); ok {
			if login, ok := reviewer["login"].(string); ok {
				event.Target = login
			} else if name, ok := reviewer["name"].(string); ok {
				event.Target = name
			}
		}

	case "ReviewRequestRemovedEvent":
		event.Kind = "review_request_removed"
		if reviewer, ok := item["requestedReviewer"].(map[string]interface{}); ok {
			if login, ok := reviewer["login"].(string); ok {
				event.Target = login
			} else if name, ok := reviewer["name"].(string); ok {
				event.Target = name
			}
		}

	case "MentionedEvent":
		event.Kind = "mentioned"
		event.Body = "User was mentioned"

	case "ReadyForReviewEvent":
		event.Kind = "ready_for_review"

	case "ConvertToDraftEvent":
		event.Kind = "convert_to_draft"

	case "ClosedEvent":
		event.Kind = "closed"

	case "ReopenedEvent":
		event.Kind = "reopened"

	case "MergedEvent":
		event.Kind = "merged"

	default:
		// Unknown event type
		return nil
	}

	return event
}

// writeAccessFromAssociation calculates write access from association
func (c *Client) writeAccessFromAssociation(ctx context.Context, owner, repo, user, association string) int {
	if user == "" {
		return WriteAccessNA
	}

	switch association {
	case "OWNER", "COLLABORATOR":
		return WriteAccessDefinitely
	case "MEMBER":
		// For MEMBER, we'd need an additional API call to check permissions
		// This is the one case where GraphQL doesn't give us everything
		// For now, return likely
		return WriteAccessLikely
	case "CONTRIBUTOR", "NONE", "FIRST_TIME_CONTRIBUTOR", "FIRST_TIMER":
		return WriteAccessUnlikely
	default:
		return WriteAccessNA
	}
}

// extractRequiredChecksFromGraphQL gets required checks from GraphQL response
func (c *Client) extractRequiredChecksFromGraphQL(data *graphQLPullRequestComplete) []string {
	checkMap := make(map[string]bool)

	// From refUpdateRule
	if data.BaseRef.RefUpdateRule != nil {
		for _, check := range data.BaseRef.RefUpdateRule.RequiredStatusCheckContexts {
			checkMap[check] = true
		}
	}

	// From branchProtectionRule
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

// calculateTestStateFromGraphQL determines test state from check runs
func (c *Client) calculateTestStateFromGraphQL(data *graphQLPullRequestComplete) string {
	if data.HeadRef.Target.StatusCheckRollup == nil {
		return ""
	}

	var hasFailure, hasRunning, hasQueued bool

	for _, node := range data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes {
		if node.TypeName != "CheckRun" {
			continue
		}

		// Only consider test-related check runs
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
		}

		switch strings.ToLower(node.Conclusion) {
		case "failure", "timed_out", "action_required":
			hasFailure = true
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