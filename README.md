# prx

A Go library for efficiently retrieving rich details about GitHub pull requests, including all events in chronological order.

## Installation

```bash
go get github.com/ready-to-review/prx
```

## CLI Usage

The repository includes a command-line tool that outputs pull request data as JSON:

```bash
# Install the CLI tool
go install github.com/ready-to-review/prx/cmd/prx@latest

# Authenticate with GitHub CLI (required)
gh auth login

# Fetch pull request data
prx https://github.com/golang/go/pull/12345
```

The CLI outputs a single JSON object containing the pull request metadata and all events:

```bash
# Extract just the events
prx https://github.com/golang/go/pull/12345 | jq '.events[]'

# Count events by kind
prx https://github.com/golang/go/pull/12345 | jq '.events[].kind' | sort | uniq -c

# Show all review comments
prx https://github.com/golang/go/pull/12345 | jq '.events[] | select(.kind == "review_comment")'

# Show PR metadata
prx https://github.com/golang/go/pull/12345 | jq '.pull_request'
```

## Library Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/ready-to-review/prx/pkg/prx"
)

func main() {
    // Create a client with your GitHub token
    token := os.Getenv("GITHUB_TOKEN")
    client := prx.NewClient(token)

    // Fetch pull request data
    ctx := context.Background()
    data, err := client.PullRequest(ctx, "owner", "repo", 123)
    if err != nil {
        log.Fatal(err)
    }

    // Access PR metadata
    fmt.Printf("PR #%d: %s\n", data.PullRequest.Number, data.PullRequest.Title)
    fmt.Printf("Author: %s\n", data.PullRequest.Author)
    fmt.Printf("State: %s\n", data.PullRequest.State)

    // Process events
    for _, event := range data.Events {
        fmt.Printf("%s: %s by %s\n", 
            event.Timestamp.Format("2006-01-02 15:04:05"),
            event.Kind,
            event.Actor)
    }
}
```

## Data Structure

### Pull Request Data

The main response contains:

```go
type PullRequestData struct {
    PullRequest PullRequest `json:"pull_request"`
    Events      []Event     `json:"events"`
}
```

### Pull Request Metadata

```go
type PullRequest struct {
    Number            int          `json:"number"`
    Title             string       `json:"title"`
    State             string       `json:"state"`
    Author            string       `json:"author"`
    AuthorAssociation string       `json:"author_association"`
    CreatedAt         time.Time    `json:"created_at"`
    UpdatedAt         time.Time    `json:"updated_at"`
    ClosedAt          *time.Time   `json:"closed_at,omitempty"`
    MergedAt          *time.Time   `json:"merged_at,omitempty"`
    MergeableState    string       `json:"mergeable_state"`
    Additions         int          `json:"additions"`
    Deletions         int          `json:"deletions"`
    ChangedFiles      int          `json:"changed_files"`
    Assignees         []string     `json:"assignees,omitempty"`
    RequestedReviewers []string    `json:"requested_reviewers,omitempty"`
    Labels            []string     `json:"labels,omitempty"`
    TestSummary       *TestSummary   `json:"test_summary,omitempty"`
    StatusSummary     *StatusSummary `json:"status_summary,omitempty"`
}

type TestSummary struct {
    Passing int `json:"passing"`
    Failing int `json:"failing"`
    Pending int `json:"pending"`
}

type StatusSummary struct {
    Success int `json:"success"`
    Failure int `json:"failure"`
    Pending int `json:"pending"`
    Neutral int `json:"neutral"`
}
```

### Event Structure

Each event has a unified structure:

```go
type Event struct {
    Kind              EventKind  `json:"kind"`
    Timestamp         time.Time  `json:"timestamp"`
    Actor             string     `json:"actor"`
    Bot               bool       `json:"bot,omitempty"`
    Targets           []string   `json:"targets,omitempty"`
    Outcome           string     `json:"outcome,omitempty"`
    Body              string     `json:"body,omitempty"`
    Question          bool       `json:"question,omitempty"`
    AuthorAssociation string     `json:"author_association,omitempty"`
}
```

### Event Examples

```json
{
  "kind": "review",
  "timestamp": "2024-01-15T10:30:00Z",
  "actor": "reviewer",
  "outcome": "approved",
  "body": "Looks good!",
  "author_association": "MEMBER"
}

{
  "kind": "comment",
  "timestamp": "2024-01-15T11:00:00Z", 
  "actor": "user123",
  "body": "Can we add tests for this?",
  "question": true,
  "targets": ["@author"]
}

{
  "kind": "labeled",
  "timestamp": "2024-01-15T09:00:00Z",
  "actor": "triager",
  "targets": ["bug", "high-priority"]
}

{
  "kind": "check_run",
  "timestamp": "2024-01-15T12:00:00Z",
  "actor": "github-actions",
  "bot": true,
  "outcome": "failure",
  "body": "test"
}
```

## Event Types

The library fetches the following event kinds:

- **commit**: All commits in the pull request
- **comment**: Issue comments on the pull request
- **review**: Review submissions (outcome: "approved", "changes_requested", "commented")
- **review_comment**: Inline code review comments
- **status_check**: CI/CD status updates (status name in `body` field, outcome: "success", "failure", "pending", "error")
- **check_run**: GitHub Actions and other check runs (check name in `body` field)
- **assigned**, **unassigned**: Assignment changes
- **review_requested**, **review_request_removed**: Review request changes
- **labeled**, **unlabeled**: Label changes
- **milestoned**, **demilestoned**: Milestone changes
- **renamed**: Title changes
- **opened**, **closed**, **reopened**, **merged**: State changes
- **head_ref_force_pushed**: Force push to the pull request branch

## Features

- **Concurrent fetching** of different event types for optimal performance
- **Automatic pagination** handling for large pull requests
- **Chronological ordering** of all events
- **Bot detection** (marks events from bots with `"bot": true`)
- **Mention extraction** (populated in `targets` field for comments/reviews)
- **Question detection** (marks comments containing questions)
- **Caching support** via `prx.NewCacheClient()` for reduced API calls
- **Structured logging** with slog
- **Retry logic** with exponential backoff for API reliability

## Caching

For applications that need to fetch the same PR data repeatedly:

```go
// Create a caching client
cacheDir := "/tmp/prx-cache"
client, err := prx.NewCacheClient(token, cacheDir)

// Fetch with caching (uses updated_at timestamp for cache invalidation)
data, err := client.PullRequest(ctx, "owner", "repo", 123, time.Now())
```

Cache files are automatically cleaned up after 20 days.

## Authentication

The library requires a GitHub personal access token or GitHub App token with:
- `repo` scope for private repositories
- `public_repo` scope for public repositories only

## License

MIT