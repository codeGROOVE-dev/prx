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
	// tokenPreviewPrefixLen is the number of characters to show at the start of a masked token.
	tokenPreviewPrefixLen = 4
	// tokenPreviewSuffixLen is the number of characters to show at the end of a masked token.
	tokenPreviewSuffixLen = 4
	// tokenPreviewMinLen is the minimum token length to show a preview.
	tokenPreviewMinLen = 8
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, http.NoBody)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// Log request details (mask token for security)
	tokenPreview := ""
	if c.token != "" {
		if len(c.token) > tokenPreviewMinLen {
			tokenPreview = c.token[:tokenPreviewPrefixLen] + "..." + c.token[len(c.token)-tokenPreviewSuffixLen:]
		} else {
			tokenPreview = "***"
		}
	}

	slog.InfoContext(ctx, "GitHub API request starting",
		"method", "GET",
		"url", apiURL,
		"headers", map[string]string{
			"Authorization": "Bearer " + tokenPreview,
			"Accept":        req.Header.Get("Accept"),
			"User-Agent":    req.Header.Get("User-Agent"),
		})

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

	// Log rate limit headers for all responses
	rateLimitHeaders := map[string]string{
		"X-RateLimit-Limit":     resp.Header.Get("X-Ratelimit-Limit"),
		"X-RateLimit-Remaining": resp.Header.Get("X-Ratelimit-Remaining"),
		"X-RateLimit-Reset":     resp.Header.Get("X-Ratelimit-Reset"),
		"X-RateLimit-Used":      resp.Header.Get("X-Ratelimit-Used"),
		"X-RateLimit-Resource":  resp.Header.Get("X-Ratelimit-Resource"),
		"Retry-After":           resp.Header.Get("Retry-After"),
	}

	slog.InfoContext(ctx, "GitHub API response received",
		"status", resp.Status,
		"url", apiURL,
		"elapsed", elapsed,
		"rate_limits", rateLimitHeaders)

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		if readErr != nil {
			body = []byte("failed to read response body")
		}

		// Log comprehensive error details including headers
		errorHeaders := make(map[string][]string)
		for key, values := range resp.Header {
			// Include relevant error-related headers
			if strings.HasPrefix(key, "X-") ||
				key == "Content-Type" ||
				key == "Date" ||
				key == "Server" ||
				key == "Retry-After" ||
				key == "X-GitHub-Request-Id" {
				errorHeaders[key] = values
			}
		}

		// Log collaborator 403 errors as warnings since they're expected for repos without push access
		if resp.StatusCode == http.StatusForbidden && strings.Contains(apiURL, "/collaborators") {
			slog.WarnContext(ctx, "GitHub API access denied",
				"status", resp.Status,
				"status_code", resp.StatusCode,
				"url", apiURL,
				"body", string(body),
				"headers", errorHeaders)
		} else {
			slog.ErrorContext(ctx, "GitHub API error",
				"status", resp.Status,
				"status_code", resp.StatusCode,
				"url", apiURL,
				"body", string(body),
				"headers", errorHeaders)
		}
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
	links := strings.SplitSeq(linkHeader, ",")
	for link := range links {
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

// collaborators fetches all users with repository access and their permission levels.
// Returns a map of username -> permission level ("admin", "write", "read", "none").
// Uses affiliation=all to include direct collaborators, org members, and outside collaborators.
func (c *githubClient) collaborators(ctx context.Context, owner, repo string) (map[string]string, error) {
	path := fmt.Sprintf("/repos/%s/%s/collaborators?affiliation=all&per_page=100", owner, repo)

	type collaborator struct {
		Login       string `json:"login"`
		Permissions struct {
			Admin    bool `json:"admin"`
			Maintain bool `json:"maintain"`
			Push     bool `json:"push"`
			Triage   bool `json:"triage"`
			Pull     bool `json:"pull"`
		} `json:"permissions"`
	}

	var collabs []collaborator
	if _, err := c.get(ctx, path, &collabs); err != nil {
		return nil, err
	}

	result := make(map[string]string, len(collabs))
	for _, collab := range collabs {
		// Determine permission level from boolean flags
		switch {
		case collab.Permissions.Admin:
			result[collab.Login] = "admin"
		case collab.Permissions.Maintain:
			result[collab.Login] = "maintain"
		case collab.Permissions.Push:
			result[collab.Login] = "write"
		case collab.Permissions.Triage:
			result[collab.Login] = "triage"
		case collab.Permissions.Pull:
			result[collab.Login] = "read"
		default:
			result[collab.Login] = "none"
		}
	}

	return result, nil
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

// githubCheckRun represents a GitHub check run from the REST API.
type githubCheckRun struct {
	Name        string    `json:"name"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Conclusion  string    `json:"conclusion"`
	Status      string    `json:"status"`
	Output      struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	} `json:"output"`
}

// githubCheckRuns represents a list of GitHub check runs.
type githubCheckRuns struct {
	CheckRuns []*githubCheckRun `json:"check_runs"`
}

// githubRuleset represents a repository ruleset from the REST API.
type githubRuleset struct {
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
