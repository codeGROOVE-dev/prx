package prevents_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ready-to-review/prevents/pkg/prevents"
)

func Example() {
	// Create a client with your GitHub token
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable not set")
	}

	client := prevents.NewClient(token)

	// Fetch events for a pull request
	ctx := context.Background()
	events, err := client.FetchPullRequestEvents(ctx, "owner", "repo", 123)
	if err != nil {
		log.Fatal(err)
	}

	// Process events
	for _, event := range events {
		fmt.Printf("%s: %s by %s\n", 
			event.Timestamp.Format("2006-01-02 15:04:05"),
			event.Type,
			event.Actor,
		)
	}
}

func ExampleClient_FetchPullRequestEvents() {
	// Create a client with custom logger
	token := os.Getenv("GITHUB_TOKEN")
	client := prevents.NewClient(token)

	// Fetch all events for PR #123
	ctx := context.Background()
	events, err := client.FetchPullRequestEvents(ctx, "golang", "go", 123)
	if err != nil {
		log.Fatal(err)
	}

	// Count events by type
	eventCounts := make(map[prevents.EventType]int)
	for _, event := range events {
		eventCounts[event.Type]++
	}

	// Print summary
	fmt.Printf("Total events: %d\n", len(events))
	for eventType, count := range eventCounts {
		fmt.Printf("%s: %d\n", eventType, count)
	}
}