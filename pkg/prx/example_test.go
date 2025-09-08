package prx_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/codeGROOVE-dev/prx/pkg/prx"
)

func Example() {
	// Create a client with your GitHub token
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable not set")
	}

	client := prx.NewClient(token)

	// Fetch events for a pull request
	ctx := context.Background()
	data, err := client.PullRequest(ctx, "owner", "repo", 123)
	if err != nil {
		log.Fatal(err)
	}

	// Show PR metadata
	fmt.Printf("PR #%d: %s\n", data.PullRequest.Number, data.PullRequest.Title)
	fmt.Printf("Author: %s (write access: %d)\n", data.PullRequest.Author, data.PullRequest.AuthorWriteAccess)
	fmt.Printf("Status: %s, Mergeable: %v\n", data.PullRequest.State, data.PullRequest.MergeableState)

	// Process events
	for _, event := range data.Events {
		fmt.Printf("%s: %s by %s\n",
			event.Timestamp.Format("2006-01-02 15:04:05"),
			event.Kind,
			event.Actor,
		)
	}
}

func ExampleClient_PullRequest() {
	// Create a client with custom logger
	token := os.Getenv("GITHUB_TOKEN")
	client := prx.NewClient(token)

	// Fetch all events for PR #123
	ctx := context.Background()
	data, err := client.PullRequest(ctx, "golang", "go", 123)
	if err != nil {
		log.Fatal(err)
	}

	// Show PR size
	fmt.Printf("PR #%d: +%d -%d in %d files\n",
		data.PullRequest.Number,
		data.PullRequest.Additions,
		data.PullRequest.Deletions,
		data.PullRequest.ChangedFiles)

	// Count events by type
	eventCounts := make(map[string]int)
	for _, event := range data.Events {
		eventCounts[event.Kind]++
	}

	// Print summary
	fmt.Printf("Total events: %d\n", len(data.Events))
	for eventKind, count := range eventCounts {
		fmt.Printf("%s: %d\n", eventKind, count)
	}
}
