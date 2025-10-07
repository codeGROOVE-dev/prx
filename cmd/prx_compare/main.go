// Package main provides a comparison utility for REST vs GraphQL PR fetching.
package main

import (
	"context"
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

const (
	defaultPRNumber       = 1359
	truncateDisplayLength = 50
)

func main() {
	var token string
	var owner string
	var repo string
	var prNumber int

	flag.StringVar(&token, "token", os.Getenv("GITHUB_TOKEN"), "GitHub token")
	flag.StringVar(&owner, "owner", "oxidecomputer", "Repository owner")
	flag.StringVar(&repo, "repo", "dropshot", "Repository name")
	flag.IntVar(&prNumber, "pr", defaultPRNumber, "Pull request number")
	flag.Parse()

	if token == "" {
		log.Fatal("GitHub token required (set GITHUB_TOKEN or use -token)")
	}

	// Both now use GraphQL, but we'll compare two fetches to ensure consistency
	fmt.Println("Fetching first time...")
	restClient := prx.NewClient(token)
	restData, err := restClient.PullRequest(context.TODO(), owner, repo, prNumber)
	if err != nil {
		log.Fatalf("First fetch failed: %v", err)
	}

	// Fetch again to compare consistency
	fmt.Println("Fetching second time...")
	graphqlClient := prx.NewClient(token)
	graphqlData, err := graphqlClient.PullRequest(context.TODO(), owner, repo, prNumber)
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
	differences, matches := compareFields(rest, graphql)

	if len(differences) > 0 {
		fmt.Println("Differences found:")
		for _, diff := range differences {
			fmt.Println(diff)
		}
	}

	fmt.Printf("\nMatching fields: %s\n", strings.Join(matches, ", "))
}

func compareFields(rest, graphql *prx.PullRequest) (differences, matches []string) {
	restVal := reflect.ValueOf(*rest)
	graphqlVal := reflect.ValueOf(*graphql)
	restType := restVal.Type()

	for i := range restVal.NumField() {
		field := restType.Field(i)
		restField := restVal.Field(i)
		graphqlField := graphqlVal.Field(i)

		// Special handling for pointer fields
		if restField.Kind() == reflect.Ptr {
			if diff := comparePointerField(field.Name, restField, graphqlField); diff != "" {
				differences = append(differences, diff)
				continue
			}
		}

		// Compare values
		if !reflect.DeepEqual(restField.Interface(), graphqlField.Interface()) {
			if field.Name == "CheckSummary" {
				compareCheckSummary(rest, graphql)
			} else {
				differences = append(differences, fmt.Sprintf("  %s: REST=%v, GraphQL=%v",
					field.Name, restField.Interface(), graphqlField.Interface()))
			}
		} else {
			matches = append(matches, field.Name)
		}
	}

	return differences, matches
}

func comparePointerField(name string, restField, graphqlField reflect.Value) string {
	if restField.IsNil() != graphqlField.IsNil() {
		return fmt.Sprintf("  %s: REST=%v, GraphQL=%v", name, restField.IsNil(), graphqlField.IsNil())
	}
	if !restField.IsNil() {
		restField.Elem()
		graphqlField.Elem()
	}
	return ""
}

func compareCheckSummary(rest, graphql *prx.PullRequest) {
	if rest.CheckSummary == nil || graphql.CheckSummary == nil {
		return
	}

	fmt.Println("  CheckSummary:")
	fmt.Printf("    REST:    Success=%d, Failing=%d, Pending=%d, Cancelled=%d, Skipped=%d, Stale=%d, Neutral=%d\n",
		len(rest.CheckSummary.Success), len(rest.CheckSummary.Failing),
		len(rest.CheckSummary.Pending), len(rest.CheckSummary.Cancelled),
		len(rest.CheckSummary.Skipped), len(rest.CheckSummary.Stale), len(rest.CheckSummary.Neutral))
	fmt.Printf("    GraphQL: Success=%d, Failing=%d, Pending=%d, Cancelled=%d, Skipped=%d, Stale=%d, Neutral=%d\n",
		len(graphql.CheckSummary.Success), len(graphql.CheckSummary.Failing),
		len(graphql.CheckSummary.Pending), len(graphql.CheckSummary.Cancelled),
		len(graphql.CheckSummary.Skipped), len(graphql.CheckSummary.Stale), len(graphql.CheckSummary.Neutral))

	compareCheckSummaryMaps(rest.CheckSummary, graphql.CheckSummary)
}

func compareCheckSummaryMaps(rest, graphql *prx.CheckSummary) {
	compareSummaryMap("Success", rest.Success, graphql.Success)
	compareSummaryMap("Failing", rest.Failing, graphql.Failing)
	compareSummaryMap("Pending", rest.Pending, graphql.Pending)
	compareSummaryMap("Cancelled", rest.Cancelled, graphql.Cancelled)
	compareSummaryMap("Skipped", rest.Skipped, graphql.Skipped)
	compareSummaryMap("Stale", rest.Stale, graphql.Stale)
	compareSummaryMap("Neutral", rest.Neutral, graphql.Neutral)
}

func compareSummaryMap(name string, rest, graphql map[string]string) {
	if len(rest) > 0 || len(graphql) > 0 {
		fmt.Printf("    %s:\n", name)
		compareStatusMaps(rest, graphql)
	}
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
				for i := range restTypeEvents {
					if i >= maxShow {
						break
					}
					e := &restTypeEvents[i]
					fmt.Printf("    - %s by %s: %s\n", e.Timestamp.Format("2006-01-02 15:04"), e.Actor, truncate(e.Body, truncateDisplayLength))
				}
			}
			if len(graphqlTypeEvents) > 0 && len(graphqlTypeEvents) <= maxShow {
				fmt.Println("  GraphQL events:")
				for i := range graphqlTypeEvents {
					if i >= maxShow {
						break
					}
					e := &graphqlTypeEvents[i]
					fmt.Printf("    - %s by %s: %s\n", e.Timestamp.Format("2006-01-02 15:04"), e.Actor, truncate(e.Body, truncateDisplayLength))
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
	for i := range events {
		counts[events[i].Kind]++
	}
	return counts
}

func groupEventsByType(events []prx.Event) map[string][]prx.Event {
	grouped := make(map[string][]prx.Event)
	for i := range events {
		grouped[events[i].Kind] = append(grouped[events[i].Kind], events[i])
	}
	return grouped
}

func extractWriteAccess(events []prx.Event) map[string]int {
	access := make(map[string]int)
	for i := range events {
		e := &events[i]
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
	for i := range events {
		e := &events[i]
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

func saveJSON(filename string, data any) {
	file, err := os.Create(filename)
	if err != nil {
		log.Printf("Failed to create %s: %v", filename, err)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close %s: %v", filename, err)
		}
	}()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		log.Printf("Failed to encode %s: %v", filename, err)
	}
}
