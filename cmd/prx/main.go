package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ready-to-review/prx/pkg/prx"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <pull-request-url>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s https://github.com/golang/go/pull/12345\n", os.Args[0])
		os.Exit(1)
	}

	prURL := os.Args[1]

	// Parse the PR URL
	owner, repo, prNumber, err := parsePRURL(prURL)
	if err != nil {
		log.Fatalf("Invalid PR URL: %v", err)
	}

	// Get GitHub token using gh auth token
	token, err := githubToken()
	if err != nil {
		log.Fatalf("Failed to get GitHub token: %v", err)
	}

	// Get user cache directory
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatalf("Failed to get user cache directory: %v", err)
	}

	// Create application-specific cache directory
	cacheDir := filepath.Join(userCacheDir, "prx")

	// Create cache client
	client, err := prx.NewCacheClient(token, cacheDir)
	if err != nil {
		log.Fatalf("Failed to create cache client: %v", err)
	}

	// Fetch events with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Use time.Now() as reference timestamp, which will invalidate the initial PR request
	// but subsequent API calls will use the PR's updated_at field
	data, err := client.PullRequest(ctx, owner, repo, prNumber, time.Now())
	if err != nil {
		log.Fatalf("Failed to fetch PR data: %v", err)
	}

	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(data); err != nil {
		log.Fatalf("Failed to encode pull request: %v", err)
	}
}

func githubToken() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run 'gh auth token': %w", err)
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", fmt.Errorf("no token returned by 'gh auth token'")
	}

	return token, nil
}

func parsePRURL(prURL string) (owner, repo string, prNumber int, err error) {
	u, err := url.Parse(prURL)
	if err != nil {
		return "", "", 0, err
	}

	if u.Host != "github.com" {
		return "", "", 0, fmt.Errorf("not a GitHub URL")
	}

	// Expected format: /owner/repo/pull/number
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pull" {
		return "", "", 0, fmt.Errorf("invalid PR URL format")
	}

	owner = parts[0]
	repo = parts[1]
	prNumber, err = strconv.Atoi(parts[3])
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid PR number: %w", err)
	}

	return owner, repo, prNumber, nil
}
