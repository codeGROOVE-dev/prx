package prx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	githubAPI = "https://api.github.com"
	// maxResponseSize limits API response size to prevent memory exhaustion.
	maxResponseSize = 10 * 1024 * 1024 // 10MB
	// maxErrorBodySize limits error response body reading for debugging.
	maxErrorBodySize = 1024
)

// GitHubAPIError represents an error response from the GitHub API.
type GitHubAPIError struct {
	Status     string
	Body       string
	URL        string
	StatusCode int
}

func (e *GitHubAPIError) Error() string {
	return fmt.Sprintf("github API error: %s", e.Status)
}

// githubClient is a client for interacting with the GitHub API.
type githubClient struct {
	client *http.Client
	token  string
	api    string
}

// doRequest performs the common HTTP request logic for GitHub API calls.
func (c *githubClient) doRequest(ctx context.Context, path string) ([]byte, *githubResponse, error) {
	apiURL := c.api + path
	slog.InfoContext(ctx, "GitHub API request starting", "method", "GET", "url", apiURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, http.NoBody)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	start := time.Now()
	resp, err := c.client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		slog.ErrorContext(ctx, "GitHub API request failed", "url", apiURL, "error", err, "elapsed", elapsed)
		return nil, nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.DebugContext(ctx, "failed to close response body", "error", closeErr, "url", apiURL)
		}
	}()

	slog.InfoContext(ctx, "GitHub API response received", "status", resp.Status, "url", apiURL, "elapsed", elapsed)

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		if readErr != nil {
			body = []byte("failed to read response body")
		}
		slog.ErrorContext(ctx, "GitHub API error", "status", resp.Status, "url", apiURL, "body", string(body))
		return nil, nil, &GitHubAPIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(body),
			URL:        apiURL,
		}
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, nil, err
	}

	// Parse Link header for pagination
	nextPageNum := 0
	linkHeader := resp.Header.Get("Link")
	links := strings.Split(linkHeader, ",")
	for _, link := range links {
		parts := strings.Split(strings.TrimSpace(link), ";")
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == `rel="next"` {
			u, err := url.Parse(strings.Trim(parts[0], "<>"))
			if err == nil {
				page := u.Query().Get("page")
				nextPageNum, err = strconv.Atoi(page)
				if err != nil {
					nextPageNum = 0
				}
			}
			break
		}
	}

	return data, &githubResponse{NextPage: nextPageNum}, nil
}

// get makes a GET request to the GitHub API and decodes the response into v.
func (c *githubClient) get(ctx context.Context, path string, v any) (*githubResponse, error) {
	data, resp, err := c.doRequest(ctx, path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, v); err != nil {
		return nil, err
	}

	return resp, nil
}

// raw makes a GET request to the GitHub API and returns the raw JSON response.
func (c *githubClient) raw(ctx context.Context, path string) (json.RawMessage, *githubResponse, error) {
	data, resp, err := c.doRequest(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	return json.RawMessage(data), resp, nil
}

// userPermission gets the permission level for a user on a repository.
// Returns "admin", "write", "read", or "none".
func (c *githubClient) userPermission(ctx context.Context, owner, repo, username string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/collaborators/%s/permission", owner, repo, username)

	var permResp struct {
		Permission string `json:"permission"`
	}

	if _, err := c.get(ctx, path, &permResp); err != nil {
		// Return the error so caller can handle it appropriately
		return "", err
	}

	return permResp.Permission, nil
}

// githubResponse wraps a GitHub API response.
type githubResponse struct {
	NextPage int
}

// githubUser represents a GitHub user.
type githubUser struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

// githubCommit represents a GitHub commit.
type githubCommit struct {
	Author struct {
		Date time.Time `json:"date"`
	} `json:"author"`
	Message string `json:"message"`
}

// githubPullRequestCommit represents a commit in a pull request.
type githubPullRequestCommit struct {
	Author *githubUser  `json:"author"`
	Commit githubCommit `json:"commit"`
}

// githubComment represents a GitHub comment.
type githubComment struct {
	User              *githubUser `json:"user"`
	CreatedAt         time.Time   `json:"created_at"`
	Body              string      `json:"body"`
	AuthorAssociation string      `json:"author_association"`
}

// githubReview represents a GitHub review.
type githubReview struct {
	User              *githubUser `json:"user"`
	SubmittedAt       time.Time   `json:"submitted_at"`
	State             string      `json:"state"`
	Body              string      `json:"body"`
	AuthorAssociation string      `json:"author_association"`
}

// githubReviewComment represents a GitHub review comment.
type githubReviewComment struct {
	User              *githubUser `json:"user"`
	CreatedAt         time.Time   `json:"created_at"`
	Body              string      `json:"body"`
	AuthorAssociation string      `json:"author_association"`
}

// githubTimelineEvent represents a GitHub timeline event.
type githubTimelineEvent struct {
	Event             string      `json:"event"`
	Actor             *githubUser `json:"actor"`
	CreatedAt         time.Time   `json:"created_at"`
	AuthorAssociation string      `json:"author_association"`
	Assignee          *githubUser `json:"assignee"`
	Label             struct {
		Name string `json:"name"`
	} `json:"label"`
	Milestone struct {
		Title string `json:"title"`
	} `json:"milestone"`
	RequestedReviewer *githubUser `json:"requested_reviewer"`
	RequestedTeam     struct {
		Name string `json:"name"`
	} `json:"requested_team"`
}

// githubStatus represents a GitHub status.
type githubStatus struct {
	Context     string      `json:"context"`     // The status check name
	Description string      `json:"description"` // Optional description
	Creator     *githubUser `json:"creator"`
	CreatedAt   time.Time   `json:"created_at"`
	State       string      `json:"state"`
	TargetURL   string      `json:"target_url"`
}

// githubCheckRun represents a GitHub check run.
type githubCheckRun struct {
	Name string `json:"name"`
	App  struct {
		Owner *githubUser `json:"owner"`
	} `json:"app"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Conclusion  string    `json:"conclusion"`
	Status      string    `json:"status"`
	HTMLURL     string    `json:"html_url"`
}

// githubCheckRuns represents a list of GitHub check runs.
type githubCheckRuns struct {
	CheckRuns []*githubCheckRun `json:"check_runs"`
}

// githubPullRequest represents a GitHub pull request.
type githubPullRequest struct {
	UpdatedAt time.Time   `json:"updated_at"`
	ClosedAt  time.Time   `json:"closed_at"`
	MergedAt  time.Time   `json:"merged_at"`
	CreatedAt time.Time   `json:"created_at"`
	MergedBy  *githubUser `json:"merged_by"`
	User      *githubUser `json:"user"`
	Mergeable *bool       `json:"mergeable"`
	Head      struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"head"`
	Body  string `json:"body"`
	State string `json:"state"`
	Title string `json:"title"`
	Base  struct {
		Ref string `json:"ref"`
	} `json:"base"`
	AuthorAssociation string `json:"author_association"`
	MergeableState    string `json:"mergeable_state"`
	Labels            []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignees          []*githubUser `json:"assignees"`
	RequestedReviewers []*githubUser `json:"requested_reviewers"`
	Deletions          int           `json:"deletions"`
	Number             int           `json:"number"`
	ChangedFiles       int           `json:"changed_files"`
	Commits            int           `json:"commits"`
	ReviewComments     int           `json:"review_comments"`
	Comments           int           `json:"comments"`
	Additions          int           `json:"additions"`
	Draft              bool          `json:"draft"`
	Merged             bool          `json:"merged"`
}

// githubBranchProtection represents branch protection settings.
type githubBranchProtection struct {
	RequiredStatusChecks *githubRequiredStatusChecks `json:"required_status_checks"`
	EnforceAdmins        struct {
		Enabled bool `json:"enabled"`
	} `json:"enforce_admins"`
	RequiredPullRequestReviews *struct {
		RequiredApprovingReviewCount int `json:"required_approving_review_count"`
	} `json:"required_pull_request_reviews"`
}

// githubRequiredStatusChecks represents required status checks from branch protection.
type githubRequiredStatusChecks struct {
	URL         string   `json:"url"`
	Strict      bool     `json:"strict"`
	Contexts    []string `json:"contexts"`
	ContextsURL string   `json:"contexts_url"`
	Checks      []struct {
		Context string `json:"context"`
		AppID   *int   `json:"app_id"`
	} `json:"checks"`
}

// githubRuleset represents a repository ruleset.
type githubRuleset struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Target string `json:"target"`
	Rules  []struct {
		Type       string `json:"type"`
		Parameters struct {
			RequiredStatusChecks []struct {
				Context string `json:"context"`
			} `json:"required_status_checks"`
		} `json:"parameters"`
	} `json:"rules"`
}

// githubWorkflows represents the list of workflows in a repository.
type githubWorkflows struct {
	Workflows []githubWorkflow `json:"workflows"`
}

// githubWorkflow represents a GitHub Actions workflow.
type githubWorkflow struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
	URL   string `json:"url"`
}

// githubWorkflowRuns represents workflow runs for a specific commit.
type githubWorkflowRuns struct {
	WorkflowRuns []githubWorkflowRun `json:"workflow_runs"`
}

// githubWorkflowRun represents a single workflow run.
type githubWorkflowRun struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	URL        string `json:"url"`
}

// githubCombinedStatus represents the combined status for a commit.
type githubCombinedStatus struct {
	State      string `json:"state"`
	TotalCount int    `json:"total_count"`
	Statuses   []struct {
		Context     string `json:"context"`
		State       string `json:"state"`
		Description string `json:"description"`
		Required    bool   `json:"required,omitempty"`
	} `json:"statuses"`
}
