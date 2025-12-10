// Package github provides a low-level client for the GitHub REST and GraphQL APIs.
package github

import (
	"bytes"
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
	// API is the default GitHub API base URL.
	API = "https://api.github.com"
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

// Error represents an error response from the GitHub API.
type Error struct {
	Status     string
	Body       string
	URL        string
	StatusCode int
}

func (e *Error) Error() string {
	return fmt.Sprintf("github API error: %s", e.Status)
}

// Response wraps a GitHub API response with pagination info.
type Response struct {
	NextPage int
}

// Client is a low-level client for interacting with the GitHub API.
type Client struct {
	HTTPClient *http.Client
	Token      string
	BaseURL    string
}

// Do performs an HTTP GET request to the GitHub API.
func (c *Client) Do(ctx context.Context, path string) ([]byte, *Response, error) {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = API
	}
	apiURL := baseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, http.NoBody)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// Log request details (mask token for security)
	tokenPreview := ""
	if c.Token != "" {
		if len(c.Token) > tokenPreviewMinLen {
			tokenPreview = c.Token[:tokenPreviewPrefixLen] + "..." + c.Token[len(c.Token)-tokenPreviewSuffixLen:]
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
	resp, err := c.HTTPClient.Do(req)
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
		return nil, nil, &Error{
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

	return data, &Response{NextPage: nextPageNum}, nil
}

// Get makes a GET request to the GitHub API and decodes the response into v.
func (c *Client) Get(ctx context.Context, path string, v any) (*Response, error) {
	data, resp, err := c.Do(ctx, path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, v); err != nil {
		return nil, err
	}

	return resp, nil
}

// Raw makes a GET request to the GitHub API and returns the raw JSON response.
func (c *Client) Raw(ctx context.Context, path string) (json.RawMessage, *Response, error) {
	data, resp, err := c.Do(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	return json.RawMessage(data), resp, nil
}

// GraphQL executes a GraphQL query against the GitHub API.
// The query and variables are sent as JSON, and the response is decoded into result.
func (c *Client) GraphQL(ctx context.Context, query string, variables map[string]any, result any) error {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = API
	}
	apiURL := baseURL + "/graphql"

	requestBody := map[string]any{
		"query":     query,
		"variables": variables,
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshaling GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("creating GraphQL request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v4+json")

	slog.InfoContext(ctx, "GitHub GraphQL request starting", "url", apiURL)

	start := time.Now()
	resp, err := c.HTTPClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		slog.ErrorContext(ctx, "GitHub GraphQL request failed", "url", apiURL, "error", err, "elapsed", elapsed)
		return fmt.Errorf("executing GraphQL request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.DebugContext(ctx, "failed to close response body", "error", closeErr, "url", apiURL)
		}
	}()

	slog.InfoContext(ctx, "GitHub GraphQL response received", "status", resp.Status, "url", apiURL, "elapsed", elapsed)

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		bodyStr := string(body)
		if readErr != nil {
			bodyStr = fmt.Sprintf("(failed to read body: %v)", readErr)
		}
		return &Error{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       bodyStr,
			URL:        apiURL,
		}
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decoding GraphQL response: %w", err)
	}

	return nil
}

// Collaborators fetches all users with repository access and their permission levels.
// Returns a map of username -> permission level ("admin", "write", "read", "none").
// Uses affiliation=all to include direct collaborators, org members, and outside collaborators.
func (c *Client) Collaborators(ctx context.Context, owner, repo string) (map[string]string, error) {
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
	if _, err := c.Get(ctx, path, &collabs); err != nil {
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
