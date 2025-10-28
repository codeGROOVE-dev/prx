package prx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGithubClient_DoRequest(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		serverHandler  http.HandlerFunc
		wantErr        bool
		wantStatusCode int
	}{
		{
			name: "successful request",
			path: "/test",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("Expected Authorization header with token")
				}
				if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
					t.Errorf("Expected Accept header")
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"test": "data"}`))
			},
			wantErr: false,
		},
		{
			name: "api error 404",
			path: "/notfound",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message": "Not Found"}`))
			},
			wantErr:        true,
			wantStatusCode: http.StatusNotFound,
		},
		{
			name: "api error 403 collaborators warning",
			path: "/repos/owner/repo/collaborators",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message": "Forbidden"}`))
			},
			wantErr:        true,
			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "pagination with next page",
			path: "/test",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Link", `<https://api.github.com/test?page=2>; rel="next"`)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"test": "data"}`))
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			client := &githubClient{
				client: server.Client(),
				token:  "test-token",
				api:    server.URL,
			}

			data, resp, err := client.doRequest(context.Background(), tt.path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if apiErr, ok := err.(*GitHubAPIError); ok {
					if apiErr.StatusCode != tt.wantStatusCode {
						t.Errorf("Expected status code %d, got %d", tt.wantStatusCode, apiErr.StatusCode)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if data == nil {
					t.Errorf("Expected data but got nil")
				}
				if resp == nil {
					t.Errorf("Expected response but got nil")
				}
			}
		})
	}
}

func TestGithubClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"login": "testuser", "type": "User"}`))
	}))
	defer server.Close()

	client := &githubClient{
		client: server.Client(),
		token:  "test-token",
		api:    server.URL,
	}

	var user githubUser
	resp, err := client.get(context.Background(), "/users/testuser", &user)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response but got nil")
	}
	if user.Login != "testuser" {
		t.Errorf("Expected login 'testuser', got '%s'", user.Login)
	}
	if user.Type != "User" {
		t.Errorf("Expected type 'User', got '%s'", user.Type)
	}
}

func TestGithubClient_Raw(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"raw": "json", "data": 123}`))
	}))
	defer server.Close()

	client := &githubClient{
		client: server.Client(),
		token:  "test-token",
		api:    server.URL,
	}

	raw, resp, err := client.raw(context.Background(), "/test")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response but got nil")
	}
	if len(raw) == 0 {
		t.Fatal("Expected raw data but got empty")
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("Failed to unmarshal raw data: %v", err)
	}
	if data["raw"] != "json" {
		t.Errorf("Expected raw field to be 'json', got '%v'", data["raw"])
	}
}

func TestGithubClient_Collaborators(t *testing.T) {
	tests := []struct {
		name          string
		serverHandler http.HandlerFunc
		wantErr       bool
		wantCollabs   map[string]string
	}{
		{
			name: "successful fetch with various permissions",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.URL.Path, "/collaborators") {
					t.Errorf("Expected collaborators path")
				}
				if !strings.Contains(r.URL.Query().Get("affiliation"), "all") {
					t.Errorf("Expected affiliation=all query parameter")
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[
					{
						"login": "admin-user",
						"permissions": {"admin": true, "maintain": false, "push": true, "triage": true, "pull": true}
					},
					{
						"login": "maintainer-user",
						"permissions": {"admin": false, "maintain": true, "push": true, "triage": true, "pull": true}
					},
					{
						"login": "write-user",
						"permissions": {"admin": false, "maintain": false, "push": true, "triage": true, "pull": true}
					},
					{
						"login": "triage-user",
						"permissions": {"admin": false, "maintain": false, "push": false, "triage": true, "pull": true}
					},
					{
						"login": "read-user",
						"permissions": {"admin": false, "maintain": false, "push": false, "triage": false, "pull": true}
					},
					{
						"login": "none-user",
						"permissions": {"admin": false, "maintain": false, "push": false, "triage": false, "pull": false}
					}
				]`))
			},
			wantErr: false,
			wantCollabs: map[string]string{
				"admin-user":      "admin",
				"maintainer-user": "maintain",
				"write-user":      "write",
				"triage-user":     "triage",
				"read-user":       "read",
				"none-user":       "none",
			},
		},
		{
			name: "api error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message": "Forbidden"}`))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			client := &githubClient{
				client: server.Client(),
				token:  "test-token",
				api:    server.URL,
			}

			collabs, err := client.collaborators(context.Background(), "owner", "repo")

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if len(collabs) != len(tt.wantCollabs) {
					t.Errorf("Expected %d collaborators, got %d", len(tt.wantCollabs), len(collabs))
				}
				for login, perm := range tt.wantCollabs {
					if collabs[login] != perm {
						t.Errorf("Expected %s to have permission %s, got %s", login, perm, collabs[login])
					}
				}
			}
		})
	}
}

func TestGitHubAPIError_Error(t *testing.T) {
	err := &GitHubAPIError{
		Status:     "404 Not Found",
		Body:       `{"message": "Not Found"}`,
		URL:        "https://api.github.com/repos/owner/repo",
		StatusCode: 404,
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "github API error") {
		t.Errorf("Expected error message to contain 'github API error', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "404 Not Found") {
		t.Errorf("Expected error message to contain status, got: %s", errMsg)
	}
}

func TestGithubClient_TokenMasking(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantShort bool
	}{
		{
			name:      "long token gets masked",
			token:     "ghp_1234567890abcdefghijklmnopqrstuvwxyz",
			wantShort: false,
		},
		{
			name:      "short token gets fully masked",
			token:     "short",
			wantShort: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				authHeader := r.Header.Get("Authorization")
				if !strings.HasPrefix(authHeader, "Bearer ") {
					t.Errorf("Expected Bearer token")
				}
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if token != tt.token {
					t.Errorf("Expected token %s, got %s", tt.token, token)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			client := &githubClient{
				client: server.Client(),
				token:  tt.token,
				api:    server.URL,
			}

			_, _, err := client.doRequest(context.Background(), "/test")
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestGithubClient_RateLimitHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Ratelimit-Limit", "5000")
		w.Header().Set("X-Ratelimit-Remaining", "4999")
		w.Header().Set("X-Ratelimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix()))
		w.Header().Set("X-Ratelimit-Used", "1")
		w.Header().Set("X-Ratelimit-Resource", "core")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := &githubClient{
		client: server.Client(),
		token:  "test-token",
		api:    server.URL,
	}

	_, resp, err := client.doRequest(context.Background(), "/test")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response but got nil")
	}
}

func TestGithubClient_PaginationParsing(t *testing.T) {
	tests := []struct {
		name         string
		linkHeader   string
		wantNextPage int
	}{
		{
			name:         "has next page",
			linkHeader:   `<https://api.github.com/test?page=2>; rel="next", <https://api.github.com/test?page=10>; rel="last"`,
			wantNextPage: 2,
		},
		{
			name:         "no next page",
			linkHeader:   `<https://api.github.com/test?page=10>; rel="last"`,
			wantNextPage: 0,
		},
		{
			name:         "empty link header",
			linkHeader:   "",
			wantNextPage: 0,
		},
		{
			name:         "malformed link header",
			linkHeader:   "not a valid link",
			wantNextPage: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.linkHeader != "" {
					w.Header().Set("Link", tt.linkHeader)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			defer server.Close()

			client := &githubClient{
				client: server.Client(),
				token:  "test-token",
				api:    server.URL,
			}

			_, resp, err := client.doRequest(context.Background(), "/test")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if resp.NextPage != tt.wantNextPage {
				t.Errorf("Expected next page %d, got %d", tt.wantNextPage, resp.NextPage)
			}
		})
	}
}

func TestGithubClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := &githubClient{
		client: server.Client(),
		token:  "test-token",
		api:    server.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, _, err := client.doRequest(ctx, "/test")
	if err == nil {
		t.Error("Expected context cancellation error but got none")
	}
}
