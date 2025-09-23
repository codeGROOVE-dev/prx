package prx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFetchRequiredChecksViaGraphQL tests the GraphQL query for fetching required checks
func TestFetchRequiredChecksViaGraphQL(t *testing.T) {
	tests := []struct {
		name           string
		owner          string
		repo           string
		branch         string
		response       interface{}
		expectedChecks []string
		expectError    bool
	}{
		{
			name:   "successful fetch with required checks",
			owner:  "testowner",
			repo:   "testrepo",
			branch: "refs/heads/main",
			response: map[string]interface{}{
				"data": map[string]interface{}{
					"repository": map[string]interface{}{
						"ref": map[string]interface{}{
							"refUpdateRule": map[string]interface{}{
								"requiredStatusCheckContexts": []string{
									"build",
									"test",
									"lint",
								},
							},
						},
					},
				},
			},
			expectedChecks: []string{"build", "test", "lint"},
			expectError:    false,
		},
		{
			name:   "no required checks configured",
			owner:  "testowner",
			repo:   "testrepo",
			branch: "refs/heads/main",
			response: map[string]interface{}{
				"data": map[string]interface{}{
					"repository": map[string]interface{}{
						"ref": map[string]interface{}{
							"refUpdateRule": nil,
						},
					},
				},
			},
			expectedChecks: nil,
			expectError:    false,
		},
		{
			name:   "GraphQL error response",
			owner:  "testowner",
			repo:   "testrepo",
			branch: "refs/heads/main",
			response: map[string]interface{}{
				"errors": []interface{}{
					map[string]interface{}{
						"message": "Repository not found",
					},
				},
			},
			expectedChecks: nil,
			expectError:    true,
		},
		{
			name:   "empty required checks array",
			owner:  "testowner",
			repo:   "testrepo",
			branch: "refs/heads/main",
			response: map[string]interface{}{
				"data": map[string]interface{}{
					"repository": map[string]interface{}{
						"ref": map[string]interface{}{
							"refUpdateRule": map[string]interface{}{
								"requiredStatusCheckContexts": []string{},
							},
						},
					},
				},
			},
			expectedChecks: []string{},
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify it's a GraphQL request
				if r.URL.Path != "/graphql" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}

				// Return the test response
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(tt.response); err != nil {
					t.Errorf("failed to encode response: %v", err)
				}
			}))
			defer server.Close()

			// Create client with test server
			gc := &githubClient{
				client: http.DefaultClient,
				token:  "test-token",
				api:    server.URL,
			}
			client := &Client{
				github: gc,
				logger: slog.Default(),
			}

			// Call the function
			ctx := context.Background()
			checks, err := client.fetchRequiredChecksViaGraphQL(ctx, tt.owner, tt.repo, tt.branch)

			// Verify results
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				// Compare checks
				if len(checks) != len(tt.expectedChecks) {
					t.Errorf("got %d checks, want %d", len(checks), len(tt.expectedChecks))
				}
				for i, check := range checks {
					if i < len(tt.expectedChecks) && check != tt.expectedChecks[i] {
						t.Errorf("check[%d] = %s, want %s", i, check, tt.expectedChecks[i])
					}
				}
			}
		})
	}
}
