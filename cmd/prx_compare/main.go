package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/codeGROOVE-dev/prx/pkg/prx"
)

func main() {
	var token string
	var owner string
	var repo string
	var prNumber int

	flag.StringVar(&token, "token", os.Getenv("GITHUB_TOKEN"), "GitHub token")
	flag.StringVar(&owner, "owner", "oxidecomputer", "Repository owner")
	flag.StringVar(&repo, "repo", "dropshot", "Repository name")
	flag.IntVar(&prNumber, "pr", 1359, "Pull request number")
	flag.Parse()

	if token == "" {
		log.Fatal("GitHub token required (set GITHUB_TOKEN or use -token)")
	}

	// Both now use GraphQL, but we'll compare two fetches to ensure consistency
	fmt.Println("Fetching first time...")
	restClient := prx.NewClient(token)
	restData, err := restClient.PullRequest(nil, owner, repo, prNumber)
	if err != nil {
		log.Fatalf("First fetch failed: %v", err)
	}

	// Fetch again to compare consistency
	fmt.Println("Fetching second time...")
	graphqlClient := prx.NewClient(token)
	graphqlData, err := graphqlClient.PullRequest(nil, owner, repo, prNumber)
	if err != nil {
		log.Fatalf("Second fetch failed: %v", err)
	}

	// Compare and report differences
	fmt.Println("\n=== COMPARISON RESULTS ===")
	comparePullRequestData(restData, graphqlData)

	// Save to files for detailed inspection
	saveJSON("rest_output.json", restData)
	saveJSON("graphql_output.json", graphqlData)
	fmt.Println("\nFull data saved to rest_output.json and graphql_output.json")
}

func comparePullRequestData(rest, graphql *prx.PullRequestData) {
	// Compare PullRequest fields
	fmt.Println("=== Pull Request Metadata ===")
	comparePullRequest(&rest.PullRequest, &graphql.PullRequest)

	// Compare Events
	fmt.Println("\n=== Events Comparison ===")
	compareEvents(rest.Events, graphql.Events)
}

func comparePullRequest(rest, graphql *prx.PullRequest) {
	// Use reflection to compare all fields
	restVal := reflect.ValueOf(*rest)
	graphqlVal := reflect.ValueOf(*graphql)
	restType := restVal.Type()

	differences := []string{}
	matches := []string{}

	for i := 0; i < restVal.NumField(); i++ {
		field := restType.Field(i)
		restField := restVal.Field(i)
		graphqlField := graphqlVal.Field(i)

		// Special handling for pointer fields
		if restField.Kind() == reflect.Ptr {
			if restField.IsNil() != graphqlField.IsNil() {
				differences = append(differences, fmt.Sprintf("  %s: REST=%v, GraphQL=%v",
					field.Name, restField.IsNil(), graphqlField.IsNil()))
				continue
			}
			if !restField.IsNil() {
				restField = restField.Elem()
				graphqlField = graphqlField.Elem()
			}
		}

		// Compare values
		if !reflect.DeepEqual(restField.Interface(), graphqlField.Interface()) {
			// Special handling for CheckSummary and ApprovalSummary
			if field.Name == "CheckSummary" || field.Name == "ApprovalSummary" {
				fmt.Printf("  %s:\n", field.Name)
				if field.Name == "CheckSummary" && rest.CheckSummary != nil && graphql.CheckSummary != nil {
					fmt.Printf("    REST:    Success=%d, Failing=%d, Pending=%d, Cancelled=%d, Skipped=%d, Stale=%d, Neutral=%d\n",
						len(rest.CheckSummary.Success), len(rest.CheckSummary.Failing),
						len(rest.CheckSummary.Pending), len(rest.CheckSummary.Cancelled),
						len(rest.CheckSummary.Skipped), len(rest.CheckSummary.Stale), len(rest.CheckSummary.Neutral))
					fmt.Printf("    GraphQL: Success=%d, Failing=%d, Pending=%d, Cancelled=%d, Skipped=%d, Stale=%d, Neutral=%d\n",
						len(graphql.CheckSummary.Success), len(graphql.CheckSummary.Failing),
						len(graphql.CheckSummary.Pending), len(graphql.CheckSummary.Cancelled),
						len(graphql.CheckSummary.Skipped), len(graphql.CheckSummary.Stale), len(graphql.CheckSummary.Neutral))

					// Compare status maps
					if len(rest.CheckSummary.Success) > 0 || len(graphql.CheckSummary.Success) > 0 {
						fmt.Println("    Success:")
						compareStatusMaps(rest.CheckSummary.Success, graphql.CheckSummary.Success)
					}
					if len(rest.CheckSummary.Failing) > 0 || len(graphql.CheckSummary.Failing) > 0 {
						fmt.Println("    Failing:")
						compareStatusMaps(rest.CheckSummary.Failing, graphql.CheckSummary.Failing)
					}
					if len(rest.CheckSummary.Pending) > 0 || len(graphql.CheckSummary.Pending) > 0 {
						fmt.Println("    Pending:")
						compareStatusMaps(rest.CheckSummary.Pending, graphql.CheckSummary.Pending)
					}
					if len(rest.CheckSummary.Cancelled) > 0 || len(graphql.CheckSummary.Cancelled) > 0 {
						fmt.Println("    Cancelled:")
						compareStatusMaps(rest.CheckSummary.Cancelled, graphql.CheckSummary.Cancelled)
					}
					if len(rest.CheckSummary.Skipped) > 0 || len(graphql.CheckSummary.Skipped) > 0 {
						fmt.Println("    Skipped:")
						compareStatusMaps(rest.CheckSummary.Skipped, graphql.CheckSummary.Skipped)
					}
					if len(rest.CheckSummary.Stale) > 0 || len(graphql.CheckSummary.Stale) > 0 {
						fmt.Println("    Stale:")
						compareStatusMaps(rest.CheckSummary.Stale, graphql.CheckSummary.Stale)
					}
					if len(rest.CheckSummary.Neutral) > 0 || len(graphql.CheckSummary.Neutral) > 0 {
						fmt.Println("    Neutral:")
						compareStatusMaps(rest.CheckSummary.Neutral, graphql.CheckSummary.Neutral)
					}
				}
			} else {
				differences = append(differences, fmt.Sprintf("  %s: REST=%v, GraphQL=%v",
					field.Name, restField.Interface(), graphqlField.Interface()))
			}
		} else {
			matches = append(matches, field.Name)
		}
	}

	if len(differences) > 0 {
		fmt.Println("Differences found:")
		for _, diff := range differences {
			fmt.Println(diff)
		}
	}

	fmt.Printf("\nMatching fields: %s\n", strings.Join(matches, ", "))
}

func compareStatusMaps(rest, graphql map[string]string) {
	allKeys := make(map[string]bool)
	for k := range rest {
		allKeys[k] = true
	}
	for k := range graphql {
		allKeys[k] = true
	}

	for k := range allKeys {
		restVal := rest[k]
		graphqlVal := graphql[k]
		if restVal != graphqlVal {
			fmt.Printf("      %s:\n", k)
			fmt.Printf("        REST:    %q\n", restVal)
			fmt.Printf("        GraphQL: %q\n", graphqlVal)
		}
	}
}

func compareEvents(restEvents, graphqlEvents []prx.Event) {
	// Count events by type
	restCounts := countEventsByType(restEvents)
	graphqlCounts := countEventsByType(graphqlEvents)

	fmt.Println("Event counts by type:")
	allTypes := make(map[string]bool)
	for k := range restCounts {
		allTypes[k] = true
	}
	for k := range graphqlCounts {
		allTypes[k] = true
	}

	var types []string
	for t := range allTypes {
		types = append(types, t)
	}
	sort.Strings(types)

	for _, eventType := range types {
		restCount := restCounts[eventType]
		graphqlCount := graphqlCounts[eventType]
		if restCount != graphqlCount {
			fmt.Printf("  %s: REST=%d, GraphQL=%d ❌\n", eventType, restCount, graphqlCount)
		} else {
			fmt.Printf("  %s: %d ✓\n", eventType, restCount)
		}
	}

	// Total events
	fmt.Printf("\nTotal events: REST=%d, GraphQL=%d\n", len(restEvents), len(graphqlEvents))

	// Check for missing events
	fmt.Println("\n=== Event Details Comparison ===")

	// Group events by type for detailed comparison
	restByType := groupEventsByType(restEvents)
	graphqlByType := groupEventsByType(graphqlEvents)

	for _, eventType := range types {
		restTypeEvents := restByType[eventType]
		graphqlTypeEvents := graphqlByType[eventType]

		if len(restTypeEvents) != len(graphqlTypeEvents) {
			fmt.Printf("\n%s events differ:\n", eventType)
			fmt.Printf("  REST has %d events\n", len(restTypeEvents))
			fmt.Printf("  GraphQL has %d events\n", len(graphqlTypeEvents))

			// Show first few differences
			maxShow := 3
			if len(restTypeEvents) > 0 && len(restTypeEvents) <= maxShow {
				fmt.Println("  REST events:")
				for i, e := range restTypeEvents {
					if i >= maxShow {
						break
					}
					fmt.Printf("    - %s by %s: %s\n", e.Timestamp.Format("2006-01-02 15:04"), e.Actor, truncate(e.Body, 50))
				}
			}
			if len(graphqlTypeEvents) > 0 && len(graphqlTypeEvents) <= maxShow {
				fmt.Println("  GraphQL events:")
				for i, e := range graphqlTypeEvents {
					if i >= maxShow {
						break
					}
					fmt.Printf("    - %s by %s: %s\n", e.Timestamp.Format("2006-01-02 15:04"), e.Actor, truncate(e.Body, 50))
				}
			}
		}
	}

	// Check WriteAccess preservation
	fmt.Println("\n=== Write Access Comparison ===")
	restWriteAccess := extractWriteAccess(restEvents)
	graphqlWriteAccess := extractWriteAccess(graphqlEvents)

	for actor, restAccess := range restWriteAccess {
		graphqlAccess := graphqlWriteAccess[actor]
		if restAccess != graphqlAccess {
			fmt.Printf("  %s: REST=%d, GraphQL=%d\n", actor, restAccess, graphqlAccess)
		}
	}

	// Check Bot detection
	fmt.Println("\n=== Bot Detection Comparison ===")
	restBots := extractBots(restEvents)
	graphqlBots := extractBots(graphqlEvents)

	allBotActors := make(map[string]bool)
	for actor := range restBots {
		allBotActors[actor] = true
	}
	for actor := range graphqlBots {
		allBotActors[actor] = true
	}

	for actor := range allBotActors {
		restIsBot := restBots[actor]
		graphqlIsBot := graphqlBots[actor]
		if restIsBot != graphqlIsBot {
			fmt.Printf("  %s: REST=%v, GraphQL=%v\n", actor, restIsBot, graphqlIsBot)
		}
	}
}

func countEventsByType(events []prx.Event) map[string]int {
	counts := make(map[string]int)
	for _, e := range events {
		counts[e.Kind]++
	}
	return counts
}

func groupEventsByType(events []prx.Event) map[string][]prx.Event {
	grouped := make(map[string][]prx.Event)
	for _, e := range events {
		grouped[e.Kind] = append(grouped[e.Kind], e)
	}
	return grouped
}

func extractWriteAccess(events []prx.Event) map[string]int {
	access := make(map[string]int)
	for _, e := range events {
		if e.Actor != "" && e.WriteAccess != 0 {
			// Keep the highest access level seen
			if current, exists := access[e.Actor]; !exists || e.WriteAccess > current {
				access[e.Actor] = e.WriteAccess
			}
		}
	}
	return access
}

func extractBots(events []prx.Event) map[string]bool {
	bots := make(map[string]bool)
	for _, e := range events {
		if e.Actor != "" {
			bots[e.Actor] = e.Bot
		}
	}
	return bots
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func saveJSON(filename string, data interface{}) {
	file, err := os.Create(filename)
	if err != nil {
		log.Printf("Failed to create %s: %v", filename, err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		log.Printf("Failed to encode %s: %v", filename, err)
	}
}