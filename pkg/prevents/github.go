package prevents

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
)

// githubAPI defines the interface for GitHub API operations.
// This interface makes the client testable by allowing mock implementations.
type githubAPIClient interface {
	get(ctx context.Context, path string, v any) (*githubResponse, error)
}

// githubClient is a client for interacting with the GitHub API.
type githubClient struct {
	client *http.Client
	token  string
	api    string
}

// Ensure githubClient implements githubAPIClient
var _ githubAPIClient = (*githubClient)(nil)

// newGithubClient creates a new githubClient.
func newGithubClient(client *http.Client, token string) *githubClient {
	return &githubClient{client: client, token: token, api: githubAPI}
}

// get makes a GET request to the GitHub API and decodes the response into v.
func (c *githubClient) get(ctx context.Context, path string, v any) (*githubResponse, error) {
	url := c.api + path
	slog.Info("API call", "url", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API error: %s", resp.Status)
	}

	// Limit response size to prevent memory exhaustion attacks
	const maxResponseSize = 10 * 1024 * 1024 // 10MB
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, v); err != nil {
		return nil, err
	}

	return &githubResponse{
		NextPage: getNextPage(resp.Header.Get("Link")),
	}, nil
}

// githubResponse wraps a GitHub API response.
type githubResponse struct {
	NextPage int
}

// getNextPage extracts the next page number from the Link header.
func getNextPage(linkHeader string) int {
	links := strings.Split(linkHeader, ",")
	for _, link := range links {
		parts := strings.Split(strings.TrimSpace(link), ";")
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == `rel="next"` {
			u, err := url.Parse(strings.Trim(parts[0], "<>"))
			if err != nil {
				return 0
			}
			page := u.Query().Get("page")
			p, _ := strconv.Atoi(page)
			return p
		}
	}
	return 0
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
	Event     string      `json:"event"`
	Actor     *githubUser `json:"actor"`
	CreatedAt time.Time   `json:"created_at"`
	Assignee  *githubUser `json:"assignee"`
	Label     struct {
		Name string `json:"name"`
	} `json:"label"`
	Milestone struct {
		Title string `json:"title"`
	} `json:"milestone"`
	Reviewer      *githubUser `json:"reviewer"`
	RequestedTeam struct {
		Name string `json:"name"`
	} `json:"requested_team"`
}

// githubStatus represents a GitHub status.
type githubStatus struct {
	Creator   *githubUser `json:"creator"`
	CreatedAt time.Time   `json:"created_at"`
	State     string      `json:"state"`
}

// githubCheckRun represents a GitHub check run.
type githubCheckRun struct {
	App struct {
		Owner *githubUser `json:"owner"`
	} `json:"app"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Conclusion  string    `json:"conclusion"`
}

// githubCheckRuns represents a list of GitHub check runs.
type githubCheckRuns struct {
	CheckRuns []*githubCheckRun `json:"check_runs"`
}

// githubPullRequest represents a GitHub pull request.
type githubPullRequest struct {
	Number    int         `json:"number"`
	Title     string      `json:"title"`
	Body      string      `json:"body"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	User      *githubUser `json:"user"`
	Merged    bool        `json:"merged"`
	MergedAt  time.Time   `json:"merged_at"`
	MergedBy  *githubUser `json:"merged_by"`
	State     string      `json:"state"`
	ClosedAt  time.Time   `json:"closed_at"`
	Head      struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	AuthorAssociation string `json:"author_association"`
	Mergeable         *bool  `json:"mergeable"`       // Can be true, false, or null
	MergeableState    string `json:"mergeable_state"` // "clean", "dirty", "blocked", "unstable", "unknown"
	Draft             bool   `json:"draft"`           // Whether the PR is a draft
	Additions         int    `json:"additions"`       // Lines added
	Deletions         int    `json:"deletions"`       // Lines removed
	ChangedFiles      int    `json:"changed_files"`   // Number of files changed
	Commits           int    `json:"commits"`         // Number of commits
	ReviewComments    int    `json:"review_comments"` // Number of review comments
	Comments          int    `json:"comments"`        // Number of issue comments
}
