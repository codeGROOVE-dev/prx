# prevents

A Go library for fetching all GitHub pull request events in chronological order.

## Installation

```bash
go get github.com/ready-to-review/prevents
```

## CLI Usage

The repository includes a command-line tool that outputs PR events as JSON:

```bash
# Install the CLI tool
go install github.com/ready-to-review/prevents/cmd/prevents@latest

# Authenticate with GitHub CLI (required)
gh auth login

# Fetch events for a pull request
prevents https://github.com/golang/go/pull/12345
```

The CLI outputs one JSON object per line, making it easy to process with tools like `jq`:

```bash
# Count events by type
prevents https://github.com/golang/go/pull/12345 | jq -r .type | sort | uniq -c

# Show all review comments
prevents https://github.com/golang/go/pull/12345 | jq 'select(.type == "review_comment")'
```

## Library Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/ready-to-review/prevents/pkg/prevents"
)

func main() {
    // Create a client with your GitHub token
    token := os.Getenv("GITHUB_TOKEN")
    client := prevents.NewClient(token)

    // Fetch events for a pull request
    ctx := context.Background()
    events, err := client.FetchPullRequestEvents(ctx, "owner", "repo", 123)
    if err != nil {
        log.Fatal(err)
    }

    // Process events
    for _, event := range events {
        fmt.Printf("%s: %s\n", event.Timestamp, event.Description)
    }
}
```

## Event Structure

Each event has a simple, unified structure:

```json
{
  "type": "review",
  "timestamp": "2024-01-15T10:30:00Z",
  "actor": "username",
  "bot": true,              // Only present and true for bot accounts
  "targets": ["user1"],     // Users/items affected by the action
  "outcome": "approved",    // For reviews and checks
  "body": "Looks good!"     // For comments and reviews
}
```

### Targets Field Examples

The `targets` field contains who or what was affected by the action:

- **Assignments**: `{"type": "assigned", "actor": "manager", "targets": ["developer1"]}`
- **Review Requests**: `{"type": "review_requested", "actor": "author", "targets": ["reviewer1"]}`
- **Labels**: `{"type": "labeled", "actor": "triager", "targets": ["bug"]}`
- **Milestones**: `{"type": "milestoned", "actor": "pm", "targets": ["v2.0 Release"]}`

## Event Types

The library fetches the following event types:

- **Commits**: All commits in the pull request (body contains commit message)
- **Comments**: Issue comments on the pull request (body contains comment text)
- **Reviews**: Review submissions (outcome: "approved", "changes_requested", "commented"; body contains review text)
- **Review Comments**: Inline code review comments (body contains comment text)
- **Status Checks**: CI/CD status updates (outcome: "success", "failure", "pending", "error")
- **Check Runs**: GitHub Actions and other check run results (outcome: "success", "failure", "neutral", "cancelled", "skipped", "timed_out", "action_required")
- **Timeline Events**: Assignments, labels, milestones, review requests, etc.
- **PR State Changes**: Opened, closed, merged, reopened

## Features

- Concurrent fetching of different event types for better performance
- Automatic pagination handling for large pull requests
- Events returned in chronological order
- Bot detection (marks events from bots with `"bot": true` - detects GitHub App bots, usernames ending with "-bot", "[bot]", or "-robot")
- Structured logging support
- Minimal external dependencies

## Authentication

The library requires a GitHub personal access token or GitHub App token with the following permissions:
- `repo` scope for private repositories
- `public_repo` scope for public repositories only

## License

MIT