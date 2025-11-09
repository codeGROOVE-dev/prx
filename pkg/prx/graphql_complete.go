package prx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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
				__typename
				login
				... on User {
					id
				}
				... on Bot {
					id
				}
			}

			mergedBy {
				__typename
				login
				... on User {
					id
				}
				... on Bot {
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

			reviewRequests(first: 100) {
				nodes {
					requestedReviewer {
						... on User {
							login
							id
						}
						... on Team {
							name
							id
						}
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
											__typename
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
						__typename
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
							outdated
							authorAssociation
							author {
								__typename
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
						__typename
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
							__typename
							login
							... on User {
								id
							}
							... on Bot {
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
							__typename
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
							__typename
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
							__typename
							login
						}
					}
					... on MilestonedEvent {
						id
						createdAt
						milestoneTitle
						actor {
							__typename
							login
						}
					}
					... on DemilestonedEvent {
						id
						createdAt
						milestoneTitle
						actor {
							__typename
							login
						}
					}
					... on ReviewRequestedEvent {
						id
						createdAt
						actor {
							__typename
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
							__typename
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
							__typename
							login
						}
					}
					... on ReopenedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on MergedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on MentionedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on ReadyForReviewEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on ConvertToDraftEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on AutoMergeEnabledEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on AutoMergeDisabledEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on ReviewDismissedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
						dismissalMessage
					}
					... on HeadRefDeletedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on RenamedTitleEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
						previousTitle
						currentTitle
					}
					... on BaseRefChangedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on BaseRefForcePushedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on HeadRefForcePushedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on HeadRefRestoredEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on LockedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on UnlockedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on AddedToMergeQueueEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on RemovedFromMergeQueueEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on AutomaticBaseChangeSucceededEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on AutomaticBaseChangeFailedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on ConnectedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on DisconnectedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on CrossReferencedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on ReferencedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on SubscribedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on UnsubscribedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on DeployedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on DeploymentEnvironmentChangedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on PinnedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on UnpinnedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on TransferredEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on UserBlockedEvent {
						id
						createdAt
						actor {
							__typename
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
		return nil, errors.New("cannot access GitHub client for GraphQL")
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

	// Process events: filter, sort by timestamp, and upgrade write access
	events = filterEvents(events)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	upgradeWriteAccess(events)

	// Calculate test state
	testState := c.calculateTestStateFromGraphQL(data)

	// Finalize PR data
	finalizePullRequest(&pr, events, requiredChecks, testState)

	return &PullRequestData{
		PullRequest: pr,
		Events:      events,
	}, nil
}

// executePaginatedGraphQL handles pagination for large PRs.
func (c *Client) executePaginatedGraphQL(
	ctx context.Context, gc *githubClient, owner, repo string, prNumber int,
) (*graphQLPullRequestComplete, error) {
	variables := map[string]any{
		"owner":  owner,
		"repo":   repo,
		"number": prNumber,
	}

	requestBody := map[string]any{
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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.WarnContext(ctx, "failed to close response body", "error", err)
		}
	}()

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
		// Extract error messages for clearer output
		var errMsgs []string
		var hasPermissionError bool
		for _, e := range result.Errors {
			errMsgs = append(errMsgs, e.Message)
			// Check for common permission-related error messages
			msg := strings.ToLower(e.Message)
			if strings.Contains(msg, "not accessible by integration") ||
				strings.Contains(msg, "resource not accessible") ||
				strings.Contains(msg, "forbidden") ||
				strings.Contains(msg, "insufficient permissions") ||
				strings.Contains(msg, "requires authentication") {
				hasPermissionError = true
			}
		}

		// Check if we got the core PR data despite errors - GraphQL can return partial data
		errStr := strings.Join(errMsgs, "; ")
		if result.Data.Repository.PullRequest.Number == 0 {
			// No PR data returned, this is a fatal error
			if hasPermissionError {
				return nil, fmt.Errorf(
					"fetching PR %s/%s#%d via GraphQL failed due to insufficient permissions: %s "+
						"(note: some fields like branchProtectionRule or refUpdateRule may require push access "+
						"even on public repositories; check token scopes or try using a token with 'repo' or 'public_repo' scope)",
					owner, repo, prNumber, errStr)
			}
			return nil, fmt.Errorf("fetching PR %s/%s#%d via GraphQL: %s", owner, repo, prNumber, errStr)
		}

		// We got PR data, just log the errors as warnings and continue
		if hasPermissionError {
			c.logger.WarnContext(ctx, "GraphQL query returned permission errors but PR data was retrieved - some fields may be missing",
				"owner", owner,
				"repo", repo,
				"pr", prNumber,
				"errors", errStr,
				"note", "fields like branchProtectionRule or refUpdateRule require push access")
		} else {
			c.logger.WarnContext(ctx, "GraphQL query returned errors but PR data was retrieved - some fields may be missing",
				"owner", owner,
				"repo", repo,
				"pr", prNumber,
				"errors", errStr)
		}
		// Continue processing with partial data
	}

	c.logger.InfoContext(ctx, "GraphQL query completed",
		"cost", result.Data.RateLimit.Cost,
		"remaining", result.Data.RateLimit.Remaining,
		"limit", result.Data.RateLimit.Limit)

	// TODO: Handle pagination if needed for commits, reviews, timeline, comments
	// For now, we fetch first 100 of each which should cover most PRs

	return &result.Data.Repository.PullRequest, nil
}

// graphQLCompleteResponse represents the complete GraphQL response.
//
//nolint:govet // fieldalignment: Complex nested anonymous struct for JSON unmarshaling - reordering would make it unreadable
type graphQLCompleteResponse struct {
	Data struct {
		Repository struct {
			PullRequest graphQLPullRequestComplete `json:"pullRequest"`
		} `json:"repository"`
		RateLimit struct {
			ResetAt   time.Time `json:"resetAt"`
			Cost      int       `json:"cost"`
			Remaining int       `json:"remaining"`
			Limit     int       `json:"limit"`
		} `json:"rateLimit"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// graphQLPullRequestComplete includes ALL fields we need.
//
//nolint:govet // fieldalignment: Complex nested anonymous struct for JSON unmarshaling - reordering would make it unreadable
type graphQLPullRequestComplete struct {
	// 16-byte fields
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
	Author    graphQLActor `json:"author"`
	// 8-byte pointer fields
	ClosedAt *time.Time    `json:"closedAt"`
	MergedAt *time.Time    `json:"mergedAt"`
	MergedBy *graphQLActor `json:"mergedBy"`
	// 16-byte string fields
	ID                string `json:"id"`
	Title             string `json:"title"`
	Body              string `json:"body"`
	State             string `json:"state"`
	Mergeable         string `json:"mergeable"`
	MergeStateStatus  string `json:"mergeStateStatus"`
	AuthorAssociation string `json:"authorAssociation"`
	// 8-byte int fields
	Number       int `json:"number"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
	ChangedFiles int `json:"changedFiles"`
	// 1-byte bool fields
	IsDraft bool `json:"isDraft"`

	Assignees struct {
		Nodes []graphQLActor `json:"nodes"`
	} `json:"assignees"`

	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`

	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Login string `json:"login,omitempty"`
				Name  string `json:"name,omitempty"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`

	BaseRef struct {
		RefUpdateRule *struct {
			RequiredStatusCheckContexts []string `json:"requiredStatusCheckContexts"`
		} `json:"refUpdateRule"`
		BranchProtectionRule *struct {
			RequiredStatusCheckContexts  []string `json:"requiredStatusCheckContexts"`
			RequiredApprovingReviewCount int      `json:"requiredApprovingReviewCount"`
			RequiresStatusChecks         bool     `json:"requiresStatusChecks"`
		} `json:"branchProtectionRule"`
		Target struct {
			OID string `json:"oid"`
		} `json:"target"`
		Name string `json:"name"`
	} `json:"baseRef"`

	HeadRef struct {
		Target struct {
			//nolint:govet // fieldalignment: Anonymous struct for GraphQL response - reordering fields would break JSON unmarshaling
			StatusCheckRollup *struct {
				Contexts struct {
					Nodes []graphQLStatusCheckNode `json:"nodes"`
				} `json:"contexts"`
				State string `json:"state"`
			} `json:"statusCheckRollup"`
			OID string `json:"oid"`
		} `json:"target"`
		Name string `json:"name"`
	} `json:"headRef"`

	Commits struct {
		PageInfo graphQLPageInfo `json:"pageInfo"`
		Nodes    []struct {
			Commit struct {
				CommittedDate time.Time `json:"committedDate"`
				Author        struct {
					User  *graphQLActor `json:"user"`
					Name  string        `json:"name"`
					Email string        `json:"email"`
				} `json:"author"`
				OID     string `json:"oid"`
				Message string `json:"message"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`

	Reviews struct {
		PageInfo graphQLPageInfo `json:"pageInfo"`
		Nodes    []struct {
			ID                string       `json:"id"`
			State             string       `json:"state"`
			Body              string       `json:"body"`
			CreatedAt         time.Time    `json:"createdAt"`
			SubmittedAt       *time.Time   `json:"submittedAt"`
			AuthorAssociation string       `json:"authorAssociation"`
			Author            graphQLActor `json:"author"`
		} `json:"nodes"`
	} `json:"reviews"`

	ReviewThreads struct {
		Nodes []struct {
			Comments struct {
				Nodes []struct {
					CreatedAt         time.Time    `json:"createdAt"`
					Author            graphQLActor `json:"author"`
					ID                string       `json:"id"`
					Body              string       `json:"body"`
					Outdated          bool         `json:"outdated"`
					AuthorAssociation string       `json:"authorAssociation"`
				} `json:"nodes"`
			} `json:"comments"`
			IsResolved bool `json:"isResolved"`
			IsOutdated bool `json:"isOutdated"`
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
		PageInfo graphQLPageInfo  `json:"pageInfo"`
		Nodes    []map[string]any `json:"nodes"`
	} `json:"timelineItems"`
}

// graphQLActor represents any GitHub actor (User, Bot, Organization).
type graphQLActor struct {
	Login string `json:"login"`
	ID    string `json:"id,omitempty"`
	Type  string `json:"type,omitempty"`
}

// isBot determines if an actor is a bot.
func isBot(actor graphQLActor) bool {
	if actor.Login == "" {
		return false
	}

	// Check the Type field first - most reliable signal from GitHub API
	if actor.Type == "Bot" {
		return true
	}

	// Check for bot patterns in login (case-insensitive for better detection)
	login := actor.Login
	lowerLogin := strings.ToLower(login)

	// Check for common bot suffixes
	if strings.HasSuffix(login, "[bot]") ||
		strings.HasSuffix(lowerLogin, "-bot") ||
		strings.HasSuffix(lowerLogin, "_bot") ||
		strings.HasSuffix(lowerLogin, "-robot") ||
		strings.HasPrefix(lowerLogin, "bot-") {
		return true
	}

	// Check for GitHub bot account patterns
	// Many bots end with "bot" without separator (e.g., "dependabot", "renovatebot")
	if strings.HasSuffix(lowerLogin, "bot") && len(login) > 3 {
		return true
	}

	// In GraphQL, bots have different IDs than users
	// Bot IDs typically start with "BOT_" or have specific patterns
	// This is a heuristic that may need adjustment
	return strings.HasPrefix(actor.ID, "BOT_") || strings.Contains(actor.ID, "Bot")
}

// graphQLStatusCheckNode can be either CheckRun or StatusContext.
type graphQLStatusCheckNode struct {
	StartedAt   *time.Time    `json:"startedAt,omitempty"`   // CheckRun
	CompletedAt *time.Time    `json:"completedAt,omitempty"` // CheckRun
	CreatedAt   *time.Time    `json:"createdAt,omitempty"`   // StatusContext
	Creator     *graphQLActor `json:"creator,omitempty"`     // StatusContext
	App         *struct {
		Name       string `json:"name"`
		DatabaseID int    `json:"databaseId"`
	} `json:"app,omitempty"` // CheckRun
	TypeName    string `json:"__typename"`
	Name        string `json:"name,omitempty"`        // CheckRun
	Status      string `json:"status,omitempty"`      // CheckRun
	Conclusion  string `json:"conclusion,omitempty"`  // CheckRun
	DetailsURL  string `json:"detailsUrl,omitempty"`  // CheckRun
	Title       string `json:"title,omitempty"`       // CheckRun
	Text        string `json:"text,omitempty"`        // CheckRun
	Summary     string `json:"summary,omitempty"`     // CheckRun
	Context     string `json:"context,omitempty"`     // StatusContext
	State       string `json:"state,omitempty"`       // StatusContext
	Description string `json:"description,omitempty"` // StatusContext
	TargetURL   string `json:"targetUrl,omitempty"`   // StatusContext
	DatabaseID  int    `json:"databaseId,omitempty"`  // CheckRun
}

// graphQLPageInfo for pagination.
type graphQLPageInfo struct {
	EndCursor   string `json:"endCursor"`
	HasNextPage bool   `json:"hasNextPage"`
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

	// Author write access and bot detection
	if data.Author.Login != "" {
		pr.AuthorWriteAccess = c.writeAccessFromAssociation(ctx, owner, repo, data.Author.Login, data.AuthorAssociation)
		pr.AuthorBot = isBot(data.Author)
	}

	// Assignees (initialize to empty slice if none)
	pr.Assignees = make([]string, 0)
	for _, assignee := range data.Assignees.Nodes {
		pr.Assignees = append(pr.Assignees, assignee.Login)
	}

	// Labels
	for _, label := range data.Labels.Nodes {
		pr.Labels = append(pr.Labels, label.Name)
	}

	// Commits (chronologically ordered - oldest to newest)
	for _, node := range data.Commits.Nodes {
		pr.Commits = append(pr.Commits, node.Commit.OID)
	}

	// Build reviewers map from review requests and actual reviews
	pr.Reviewers = buildReviewersMap(data)

	return pr
}

// buildReviewersMap constructs a map of reviewer login to their review state.
// It combines data from review requests (pending) and actual reviews (approved/changes_requested/commented).
func buildReviewersMap(data *graphQLPullRequestComplete) map[string]ReviewState {
	reviewers := make(map[string]ReviewState)

	// First, add all requested reviewers as pending
	for _, request := range data.ReviewRequests.Nodes {
		reviewer := request.RequestedReviewer
		// Teams have "name", users have "login"
		if reviewer.Login != "" {
			reviewers[reviewer.Login] = ReviewStatePending
		} else if reviewer.Name != "" {
			reviewers[reviewer.Name] = ReviewStatePending
		}
	}

	// Then, update with actual review states (latest review wins)
	for i := range data.Reviews.Nodes {
		review := &data.Reviews.Nodes[i]
		if review.Author.Login == "" {
			continue
		}

		// Map GraphQL review state to our ReviewState
		var state ReviewState
		switch strings.ToUpper(review.State) {
		case "APPROVED":
			state = ReviewStateApproved
		case "CHANGES_REQUESTED":
			state = ReviewStateChangesRequested
		case "COMMENTED":
			state = ReviewStateCommented
		default:
			// Skip unknown states
			continue
		}

		// Update the reviewer's state (latest review wins)
		reviewers[review.Author.Login] = state
	}

	return reviewers
}

// convertGraphQLToEvents converts GraphQL data to Events.
func (c *Client) convertGraphQLToEventsComplete(ctx context.Context, data *graphQLPullRequestComplete, owner, repo string) []Event {
	var events []Event

	// PR opened event
	events = append(events, Event{
		Kind:        "pr_opened",
		Timestamp:   data.CreatedAt,
		Actor:       data.Author.Login,
		Body:        truncate(data.Body),
		Bot:         isBot(data.Author),
		WriteAccess: c.writeAccessFromAssociation(ctx, owner, repo, data.Author.Login, data.AuthorAssociation),
	})

	// Commits
	for _, node := range data.Commits.Nodes {
		event := Event{
			Kind:        "commit",
			Timestamp:   node.Commit.CommittedDate,
			Body:        node.Commit.OID,               // Commit SHA
			Description: truncate(node.Commit.Message), // Commit message
		}
		if node.Commit.Author.User != nil {
			event.Actor = node.Commit.Author.User.Login
			event.Bot = isBot(*node.Commit.Author.User)
		} else {
			event.Actor = node.Commit.Author.Name
		}
		events = append(events, event)
	}

	// Reviews
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
			Kind:        "review",
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

	// Review comments
	for i := range data.ReviewThreads.Nodes {
		thread := &data.ReviewThreads.Nodes[i]
		for j := range thread.Comments.Nodes {
			comment := &thread.Comments.Nodes[j]
			event := Event{
				Kind:        "review_comment",
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

	// Issue comments
	for _, comment := range data.Comments.Nodes {
		event := Event{
			Kind:        "comment",
			Timestamp:   comment.CreatedAt,
			Actor:       comment.Author.Login,
			Body:        truncate(comment.Body),
			Question:    containsQuestion(comment.Body),
			Bot:         isBot(comment.Author),
			WriteAccess: c.writeAccessFromAssociation(ctx, owner, repo, comment.Author.Login, comment.AuthorAssociation),
		}
		events = append(events, event)
	}

	// Status checks and check runs
	if data.HeadRef.Target.StatusCheckRollup != nil {
		for i := range data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes {
			node := &data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes[i]
			switch node.TypeName {
			case "CheckRun":
				// Create separate events for started and completed states
				// to provide visibility into test lifecycle

				// Build description (shared across all events for this check run)
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

				// Create started event if timestamp exists
				if node.StartedAt != nil {
					events = append(events, Event{
						Kind:        "check_run",
						Timestamp:   *node.StartedAt,
						Body:        node.Name,
						Outcome:     strings.ToLower(node.Status),
						Bot:         true,
						Description: description,
					})
				}

				// Create completed event if timestamp exists
				if node.CompletedAt != nil {
					events = append(events, Event{
						Kind:        "check_run",
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
					Kind:        "status_check",
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
			event.Bot = isBot(*data.MergedBy)
		}
		events = append(events, event)
	}

	return events
}

// parseGraphQLTimelineEvent parses a single timeline event.
//
//nolint:gocognit,revive,maintidx // High complexity justified - must handle all GitHub timeline event types
func (*Client) parseGraphQLTimelineEvent(_ /* ctx */ context.Context, item map[string]any, _ /* owner */, _ /* repo */ string) *Event {
	typename, ok := item["__typename"].(string)
	if !ok {
		return nil
	}

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
		if actor, ok := item["actor"].(map[string]any); ok {
			if login, ok := actor["login"].(string); ok {
				return login
			}
		}
		return "unknown"
	}

	// Helper to check if actor is a bot
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
		event.Kind = "assigned"
		if assignee, ok := item["assignee"].(map[string]any); ok {
			if login, ok := assignee["login"].(string); ok {
				event.Target = login
			}
		}

	case "UnassignedEvent":
		event.Kind = "unassigned"
		if assignee, ok := item["assignee"].(map[string]any); ok {
			if login, ok := assignee["login"].(string); ok {
				event.Target = login
			}
		}

	case "LabeledEvent":
		event.Kind = "labeled"
		if label, ok := item["label"].(map[string]any); ok {
			if name, ok := label["name"].(string); ok {
				event.Target = name
			}
		}

	case "UnlabeledEvent":
		event.Kind = "unlabeled"
		if label, ok := item["label"].(map[string]any); ok {
			if name, ok := label["name"].(string); ok {
				event.Target = name
			}
		}

	case "MilestonedEvent":
		event.Kind = "milestoned"
		if title, ok := item["milestoneTitle"].(string); ok {
			event.Target = title
		}

	case "DemilestonedEvent":
		event.Kind = "demilestoned"
		if title, ok := item["milestoneTitle"].(string); ok {
			event.Target = title
		}

	case "ReviewRequestedEvent":
		event.Kind = "review_requested"
		if reviewer, ok := item["requestedReviewer"].(map[string]any); ok {
			if login, ok := reviewer["login"].(string); ok {
				event.Target = login
			} else if name, ok := reviewer["name"].(string); ok {
				event.Target = name
			}
		}

	case "ReviewRequestRemovedEvent":
		event.Kind = "review_request_removed"
		if reviewer, ok := item["requestedReviewer"].(map[string]any); ok {
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

	case "AutoMergeEnabledEvent":
		event.Kind = "auto_merge_enabled"

	case "AutoMergeDisabledEvent":
		event.Kind = "auto_merge_disabled"

	case "ReviewDismissedEvent":
		event.Kind = "review_dismissed"
		if msg, ok := item["dismissalMessage"].(string); ok {
			event.Body = msg
		}

	case "BaseRefChangedEvent":
		event.Kind = "base_ref_changed"

	case "BaseRefForcePushedEvent":
		event.Kind = "base_ref_force_pushed"

	case "HeadRefForcePushedEvent":
		event.Kind = "head_ref_force_pushed"

	case "HeadRefDeletedEvent":
		event.Kind = "head_ref_deleted"

	case "HeadRefRestoredEvent":
		event.Kind = "head_ref_restored"

	case "RenamedTitleEvent":
		event.Kind = "renamed_title"
		if prev, ok := item["previousTitle"].(string); ok {
			if curr, ok := item["currentTitle"].(string); ok {
				event.Body = fmt.Sprintf("Renamed from %q to %q", prev, curr)
			}
		}

	case "LockedEvent":
		event.Kind = "locked"

	case "UnlockedEvent":
		event.Kind = "unlocked"

	case "AddedToMergeQueueEvent":
		event.Kind = "added_to_merge_queue"

	case "RemovedFromMergeQueueEvent":
		event.Kind = "removed_from_merge_queue"

	case "AutomaticBaseChangeSucceededEvent":
		event.Kind = "automatic_base_change_succeeded"

	case "AutomaticBaseChangeFailedEvent":
		event.Kind = "automatic_base_change_failed"

	case "ConnectedEvent":
		event.Kind = "connected"

	case "DisconnectedEvent":
		event.Kind = "disconnected"

	case "CrossReferencedEvent":
		event.Kind = "cross_referenced"

	case "ReferencedEvent":
		event.Kind = "referenced"

	case "SubscribedEvent":
		event.Kind = "subscribed"

	case "UnsubscribedEvent":
		event.Kind = "unsubscribed"

	case "DeployedEvent":
		event.Kind = "deployed"

	case "DeploymentEnvironmentChangedEvent":
		event.Kind = "deployment_environment_changed"

	case "PinnedEvent":
		event.Kind = "pinned"

	case "UnpinnedEvent":
		event.Kind = "unpinned"

	case "TransferredEvent":
		event.Kind = "transferred"

	case "UserBlockedEvent":
		event.Kind = "user_blocked"

	default:
		// Unknown event type
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
		// For MEMBER, check collaborators cache to determine actual permission level
		// Members can have various permissions (admin, write, read) so we need to check
		return c.checkCollaboratorPermission(ctx, owner, repo, user)
	case "CONTRIBUTOR", "NONE", "FIRST_TIME_CONTRIBUTOR", "FIRST_TIMER":
		return WriteAccessUnlikely
	default:
		return WriteAccessNA
	}
}

// checkCollaboratorPermission checks if a user has write access by looking them up in the collaborators list.
// Uses cache to avoid repeated API calls (4 hour TTL).
func (c *Client) checkCollaboratorPermission(ctx context.Context, owner, repo, user string) int {
	// Check cache first
	if collabs, ok := c.collaboratorsCache.get(owner, repo); ok {
		switch collabs[user] {
		case "admin", "maintain", "write":
			return WriteAccessDefinitely
		case "read", "triage", "none":
			return WriteAccessNo
		default:
			// User not in collaborators list
			return WriteAccessUnlikely
		}
	}

	// Cache miss - fetch collaborators from API
	gc, ok := c.github.(*githubClient)
	if !ok {
		// Not a real GitHub client (probably test mock) - return likely as fallback
		return WriteAccessLikely
	}

	collabs, err := gc.collaborators(ctx, owner, repo)
	if err != nil {
		// API call failed (could be 403 if no permission to list collaborators)
		c.logger.WarnContext(ctx, "failed to fetch collaborators for write access check",
			"owner", owner,
			"repo", repo,
			"user", user,
			"error", err)

		// If it's a 403 (permission denied), cache an empty result to avoid retrying
		// This prevents repeated API calls for repos where we don't have access
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
			// Cache empty collaborators map to prevent future retries
			if cacheErr := c.collaboratorsCache.set(owner, repo, make(map[string]string)); cacheErr != nil {
				c.logger.WarnContext(ctx, "failed to cache empty collaborators for 403",
					"owner", owner,
					"repo", repo,
					"error", cacheErr)
			}
		}

		return WriteAccessLikely
	}

	// Store in cache
	if err := c.collaboratorsCache.set(owner, repo, collabs); err != nil {
		// Cache write failed, just log it and continue
		c.logger.WarnContext(ctx, "failed to cache collaborators",
			"owner", owner,
			"repo", repo,
			"error", err)
	}

	switch collabs[user] {
	case "admin", "maintain", "write":
		return WriteAccessDefinitely
	case "read", "triage", "none":
		return WriteAccessNo
	default:
		// User not in collaborators list
		return WriteAccessUnlikely
	}
}

// extractRequiredChecksFromGraphQL gets required checks from GraphQL response.
func (*Client) extractRequiredChecksFromGraphQL(data *graphQLPullRequestComplete) []string {
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
		default:
			// Other status
		}

		switch strings.ToLower(node.Conclusion) {
		case "failure", "timed_out", "action_required":
			hasFailure = true
		default:
			// Other conclusion
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
