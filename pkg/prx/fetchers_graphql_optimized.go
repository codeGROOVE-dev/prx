package prx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// fetchPullRequestViaGraphQL fetches most PR data in a single GraphQL query.
// This can replace multiple REST API calls and significantly reduce API quota usage.
func (c *Client) fetchPullRequestViaGraphQL(ctx context.Context, owner, repo string, prNumber int) (*graphQLPullRequestData, error) {
	gc, ok := c.github.(*githubClient)
	if !ok {
		return nil, fmt.Errorf("cannot access GitHub client for GraphQL")
	}

	query := `
	query($owner: String!, $repo: String!, $number: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $number) {
				title
				body
				state
				createdAt
				updatedAt
				closedAt
				mergedAt
				draft
				additions
				deletions
				changedFiles
				mergeable
				mergeStateStatus
				author {
					login
				}
				mergedBy {
					login
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
					}
				}
				headRef {
					name
					target {
						... on Commit {
							oid
							status {
								state
								contexts {
									state
									context
									description
									createdAt
								}
							}
							checkSuites(first: 100) {
								nodes {
									checkRuns(first: 100) {
										nodes {
											name
											status
											conclusion
											startedAt
											completedAt
											detailsUrl
											title
											summary
										}
									}
								}
							}
						}
					}
				}
				commits(first: 100) {
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
								}
							}
						}
					}
				}
				reviews(first: 100) {
					nodes {
						state
						body
						createdAt
						submittedAt
						author {
							login
						}
					}
				}
				reviewThreads(first: 100) {
					nodes {
						comments(first: 100) {
							nodes {
								body
								createdAt
								author {
									login
								}
							}
						}
					}
				}
				timelineItems(first: 100) {
					nodes {
						__typename
						... on AssignedEvent {
							createdAt
							actor {
								login
							}
							assignee {
								... on User {
									login
								}
							}
						}
						... on LabeledEvent {
							createdAt
							label {
								name
							}
							actor {
								login
							}
						}
						... on ReviewRequestedEvent {
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
							createdAt
							actor {
								login
							}
						}
						... on ReopenedEvent {
							createdAt
							actor {
								login
							}
						}
						... on MergedEvent {
							createdAt
							actor {
								login
							}
						}
						... on IssueComment {
							body
							createdAt
							author {
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
		}
	}`

	variables := map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"number": prNumber,
	}

	requestBody := map[string]interface{}{
		"query":     query,
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

	resp, err := gc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing GraphQL request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		limitedBody := io.LimitReader(resp.Body, 1024*1024) // 1MB limit
		body, err := io.ReadAll(limitedBody)
		if err != nil {
			return nil, fmt.Errorf("GraphQL request failed with status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("GraphQL request failed with status %d: %s", resp.StatusCode, body)
	}

	var result graphQLPullRequestResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding GraphQL response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL errors: %v", result.Errors)
	}

	c.logger.InfoContext(ctx, "GraphQL query completed",
		"cost", result.Data.RateLimit.Cost,
		"remaining", result.Data.RateLimit.Remaining,
		"resetAt", result.Data.RateLimit.ResetAt)

	return &result.Data.Repository.PullRequest, nil
}

// graphQLPullRequestResponse represents the full GraphQL response structure
type graphQLPullRequestResponse struct {
	Data struct {
		Repository struct {
			PullRequest graphQLPullRequestData `json:"pullRequest"`
		} `json:"repository"`
		RateLimit struct {
			Cost      int       `json:"cost"`
			Remaining int       `json:"remaining"`
			ResetAt   time.Time `json:"resetAt"`
		} `json:"rateLimit"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// graphQLPullRequestData represents the PR data from GraphQL
type graphQLPullRequestData struct {
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	State        string     `json:"state"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	ClosedAt     *time.Time `json:"closedAt"`
	MergedAt     *time.Time `json:"mergedAt"`
	Draft        bool       `json:"draft"`
	Additions    int        `json:"additions"`
	Deletions    int        `json:"deletions"`
	ChangedFiles int        `json:"changedFiles"`
	Mergeable    string     `json:"mergeable"`
	MergeStatus  string     `json:"mergeStateStatus"`

	Author struct {
		Login string `json:"login"`
	} `json:"author"`

	MergedBy *struct {
		Login string `json:"login"`
	} `json:"mergedBy"`

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
			OID    string `json:"oid"`
			Status *struct {
				State    string `json:"state"`
				Contexts []struct {
					State       string    `json:"state"`
					Context     string    `json:"context"`
					Description string    `json:"description"`
					CreatedAt   time.Time `json:"createdAt"`
				} `json:"contexts"`
			} `json:"status"`
			CheckSuites struct {
				Nodes []struct {
					CheckRuns struct {
						Nodes []struct {
							Name        string     `json:"name"`
							Status      string     `json:"status"`
							Conclusion  string     `json:"conclusion"`
							StartedAt   *time.Time `json:"startedAt"`
							CompletedAt *time.Time `json:"completedAt"`
							DetailsURL  string     `json:"detailsUrl"`
							Title       string     `json:"title"`
							Summary     string     `json:"summary"`
						} `json:"nodes"`
					} `json:"checkRuns"`
				} `json:"nodes"`
			} `json:"checkSuites"`
		} `json:"target"`
	} `json:"headRef"`

	Commits struct {
		Nodes []struct {
			Commit struct {
				OID           string    `json:"oid"`
				Message       string    `json:"message"`
				CommittedDate time.Time `json:"committedDate"`
				Author        struct {
					Name  string `json:"name"`
					Email string `json:"email"`
					User  *struct {
						Login string `json:"login"`
					} `json:"user"`
				} `json:"author"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`

	Reviews struct {
		Nodes []struct {
			State       string     `json:"state"`
			Body        string     `json:"body"`
			CreatedAt   time.Time  `json:"createdAt"`
			SubmittedAt *time.Time `json:"submittedAt"`
			Author      struct {
				Login string `json:"login"`
			} `json:"author"`
		} `json:"nodes"`
	} `json:"reviews"`

	ReviewThreads struct {
		Nodes []struct {
			Comments struct {
				Nodes []struct {
					Body      string    `json:"body"`
					CreatedAt time.Time `json:"createdAt"`
					Author    struct {
						Login string `json:"login"`
					} `json:"author"`
				} `json:"nodes"`
			} `json:"comments"`
		} `json:"nodes"`
	} `json:"reviewThreads"`

	TimelineItems struct {
		Nodes []map[string]interface{} `json:"nodes"`
	} `json:"timelineItems"`
}

// convertGraphQLToEvents converts GraphQL PR data to our Event format
func (c *Client) convertGraphQLToEvents(ctx context.Context, data *graphQLPullRequestData, owner, repo string) []Event {
	var events []Event

	// Convert commits
	for _, node := range data.Commits.Nodes {
		event := Event{
			Kind:      "commit",
			Timestamp: node.Commit.CommittedDate,
			Body:      truncate(node.Commit.Message),
		}
		if node.Commit.Author.User != nil {
			event.Actor = node.Commit.Author.User.Login
		} else {
			event.Actor = node.Commit.Author.Name
		}
		events = append(events, event)
	}

	// Convert reviews
	for _, review := range data.Reviews.Nodes {
		if review.State == "" {
			continue
		}
		timestamp := review.CreatedAt
		if review.SubmittedAt != nil {
			timestamp = *review.SubmittedAt
		}
		event := Event{
			Kind:      "review",
			Timestamp: timestamp,
			Actor:     review.Author.Login,
			Body:      truncate(review.Body),
			Outcome:   review.State,
			Question:  containsQuestion(review.Body),
		}
		events = append(events, event)
	}

	// Convert review comments
	for _, thread := range data.ReviewThreads.Nodes {
		for _, comment := range thread.Comments.Nodes {
			event := Event{
				Kind:      "review_comment",
				Timestamp: comment.CreatedAt,
				Actor:     comment.Author.Login,
				Body:      truncate(comment.Body),
				Question:  containsQuestion(comment.Body),
			}
			events = append(events, event)
		}
	}

	// Convert status checks
	if data.HeadRef.Target.Status != nil {
		for _, status := range data.HeadRef.Target.Status.Contexts {
			event := Event{
				Kind:        "status_check",
				Timestamp:   status.CreatedAt,
				Outcome:     status.State,
				Body:        status.Context,
				Description: status.Description,
			}
			events = append(events, event)
		}
	}

	// Convert check runs
	for _, suite := range data.HeadRef.Target.CheckSuites.Nodes {
		for _, run := range suite.CheckRuns.Nodes {
			var timestamp time.Time
			var outcome string

			if run.CompletedAt != nil {
				timestamp = *run.CompletedAt
				outcome = run.Conclusion
			} else if run.StartedAt != nil {
				timestamp = *run.StartedAt
				outcome = run.Status
			} else {
				continue
			}

			event := Event{
				Kind:      "check_run",
				Timestamp: timestamp,
				Body:      run.Name,
				Outcome:   outcome,
			}

			// Add description from title/summary
			switch {
			case run.Title != "" && run.Summary != "":
				event.Description = fmt.Sprintf("%s: %s", run.Title, run.Summary)
			case run.Title != "":
				event.Description = run.Title
			case run.Summary != "":
				event.Description = run.Summary
			}

			events = append(events, event)
		}
	}

	// Convert timeline items
	for _, item := range data.TimelineItems.Nodes {
		typename, _ := item["__typename"].(string)

		switch typename {
		case "AssignedEvent":
			if createdAt, ok := item["createdAt"].(string); ok {
				if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
					event := Event{
						Kind:      "assigned",
						Timestamp: t,
					}
					if actor, ok := item["actor"].(map[string]interface{}); ok {
						event.Actor, _ = actor["login"].(string)
					}
					if assignee, ok := item["assignee"].(map[string]interface{}); ok {
						event.Target, _ = assignee["login"].(string)
					}
					events = append(events, event)
				}
			}

		case "LabeledEvent":
			if createdAt, ok := item["createdAt"].(string); ok {
				if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
					event := Event{
						Kind:      "labeled",
						Timestamp: t,
					}
					if actor, ok := item["actor"].(map[string]interface{}); ok {
						event.Actor, _ = actor["login"].(string)
					}
					if label, ok := item["label"].(map[string]interface{}); ok {
						event.Target, _ = label["name"].(string)
					}
					events = append(events, event)
				}
			}

		case "IssueComment":
			if createdAt, ok := item["createdAt"].(string); ok {
				if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
					body, _ := item["body"].(string)
					event := Event{
						Kind:      "comment",
						Timestamp: t,
						Body:      truncate(body),
						Question:  containsQuestion(body),
					}
					if author, ok := item["author"].(map[string]interface{}); ok {
						event.Actor, _ = author["login"].(string)
					}
					events = append(events, event)
				}
			}
			// Add more timeline event types as needed
		}
	}

	return events
}

// extractRequiredChecksFromGraphQL extracts all required checks from GraphQL response
func extractRequiredChecksFromGraphQL(data *graphQLPullRequestData) []string {
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