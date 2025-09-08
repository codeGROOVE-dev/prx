// Package main provides the prx command-line tool for analyzing GitHub pull requests.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx"
)

const (
	expectedURLParts = 4
	pullPathIndex    = 2
	pullPathValue    = "pull"
)

func main() {
	debug := flag.Bool("debug", false, "Enable debug logging")
	noCache := flag.Bool("no-cache", false, "Disable caching")
	flag.Parse()

	if *debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [--debug] [--no-cache] <pull-request-url>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s https://github.com/golang/go/pull/12345\n", os.Args[0])
		os.Exit(1)
	}

	prURL := flag.Arg(0)

	owner, repo, prNumber, err := parsePRURL(prURL)
	if err != nil {
		log.Printf("Invalid PR URL: %v", err)
		os.Exit(1)
	}

	token, err := githubToken()
	if err != nil {
		log.Printf("Failed to get GitHub token: %v", err)
		os.Exit(1)
	}

	var opts []prx.Option
	if *debug {
		opts = append(opts, prx.WithLogger(slog.Default()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Configure client options
	if *noCache {
		opts = append(opts, prx.WithNoCache())
	}

	client := prx.NewClient(token, opts...)
	data, err := client.PullRequest(ctx, owner, repo, prNumber)
	if err != nil {
		log.Printf("Failed to fetch PR data: %v", err)
		cancel()
		os.Exit(1) //nolint:gocritic // False positive: cancel() is called immediately before os.Exit()
	}

	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(data); err != nil {
		log.Printf("Failed to encode pull request: %v", err)
		cancel()
		os.Exit(1)
	}

	cancel() // Ensure context is cancelled before exit
}

func githubToken() (string, error) {
	cmd := exec.CommandContext(context.Background(), "gh", "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run 'gh auth token': %w", err)
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", errors.New("no token returned by 'gh auth token'")
	}

	return token, nil
}

func parsePRURL(prURL string) (owner, repo string, prNumber int, err error) { //nolint:revive // Function needs all 4 return values
	u, err := url.Parse(prURL)
	if err != nil {
		return "", "", 0, err
	}

	if u.Host != "github.com" {
		return "", "", 0, errors.New("not a GitHub URL")
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != expectedURLParts || parts[pullPathIndex] != pullPathValue {
		return "", "", 0, errors.New("invalid PR URL format")
	}

	prNumber, err = strconv.Atoi(parts[3])
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid PR number: %w", err)
	}

	return parts[0], parts[1], prNumber, nil
}
