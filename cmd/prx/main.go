package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
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

	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Printf("Failed to get user cache directory: %v", err)
		os.Exit(1)
	}

	cacheDir := filepath.Join(userCacheDir, "prx")

	var opts []prx.Option
	if *debug {
		opts = append(opts, prx.WithLogger(slog.Default()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var data *prx.PullRequestData
	if *noCache {
		client := prx.NewClient(token, opts...)
		data, err = client.PullRequest(ctx, owner, repo, prNumber)
		if err != nil {
			log.Printf("Failed to fetch PR data: %v", err)
			os.Exit(1)
		}
	} else {
		client, err := prx.NewCacheClient(token, cacheDir, opts...)
		if err != nil {
			log.Printf("Failed to create cache client: %v", err)
			os.Exit(1)
		}
		data, err = client.PullRequest(ctx, owner, repo, prNumber, time.Now())
		if err != nil {
			log.Printf("Failed to fetch PR data: %v", err)
			os.Exit(1)
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(data); err != nil {
		log.Printf("Failed to encode pull request: %v", err)
		os.Exit(1)
	}
}

func githubToken() (string, error) {
	cmd := exec.CommandContext(context.Background(), "gh", "auth", "token")
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
