package prx

import (
	"strings"
	"time"
)

// graphQLCompleteResponse represents the complete GraphQL response.
//
//nolint:govet // fieldalignment: Complex nested anonymous struct for JSON unmarshaling
type graphQLCompleteResponse struct {
	Data struct {
		Repository struct {
			PullRequest graphQLPullRequestComplete `json:"pullRequest"`
		} `json:"repository"`
		RateLimit struct {
			ResetAt   time.Time `json:"resetAt"`
			Cost      int       `json:"cost"`
			Remaining int       `json:"remaining"`
			Limit     int       `json:"limit"`
		} `json:"rateLimit"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// graphQLPullRequestComplete includes all PR fields from the GraphQL response.
//
//nolint:govet // fieldalignment: Complex nested anonymous struct for JSON unmarshaling
type graphQLPullRequestComplete struct {
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
	Author    graphQLActor `json:"author"`

	ClosedAt *time.Time    `json:"closedAt"`
	MergedAt *time.Time    `json:"mergedAt"`
	MergedBy *graphQLActor `json:"mergedBy"`

	ID                string `json:"id"`
	Title             string `json:"title"`
	Body              string `json:"body"`
	State             string `json:"state"`
	Mergeable         string `json:"mergeable"`
	MergeStateStatus  string `json:"mergeStateStatus"`
	AuthorAssociation string `json:"authorAssociation"`

	Number       int `json:"number"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
	ChangedFiles int `json:"changedFiles"`

	IsDraft bool `json:"isDraft"`

	Assignees struct {
		Nodes []graphQLActor `json:"nodes"`
	} `json:"assignees"`

	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`

	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Login string `json:"login,omitempty"`
				Name  string `json:"name,omitempty"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`

	BaseRef struct {
		RefUpdateRule *struct {
			RequiredStatusCheckContexts []string `json:"requiredStatusCheckContexts"`
		} `json:"refUpdateRule"`
		BranchProtectionRule *struct {
			RequiredStatusCheckContexts  []string `json:"requiredStatusCheckContexts"`
			RequiredApprovingReviewCount int      `json:"requiredApprovingReviewCount"`
			RequiresStatusChecks         bool     `json:"requiresStatusChecks"`
		} `json:"branchProtectionRule"`
		Target struct {
			OID string `json:"oid"`
		} `json:"target"`
		Name string `json:"name"`
	} `json:"baseRef"`

	HeadRef struct {
		Target struct {
			StatusCheckRollup *struct {
				Contexts struct {
					Nodes []graphQLStatusCheckNode `json:"nodes"`
				} `json:"contexts"`
				State string `json:"state"`
			} `json:"statusCheckRollup"`
			OID string `json:"oid"`
		} `json:"target"`
		Name string `json:"name"`
	} `json:"headRef"`

	Commits struct {
		PageInfo graphQLPageInfo `json:"pageInfo"`
		Nodes    []struct {
			Commit struct {
				CommittedDate time.Time `json:"committedDate"`
				Author        struct {
					User  *graphQLActor `json:"user"`
					Name  string        `json:"name"`
					Email string        `json:"email"`
				} `json:"author"`
				OID     string `json:"oid"`
				Message string `json:"message"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`

	Reviews struct {
		PageInfo graphQLPageInfo `json:"pageInfo"`
		Nodes    []struct {
			ID                string       `json:"id"`
			State             string       `json:"state"`
			Body              string       `json:"body"`
			CreatedAt         time.Time    `json:"createdAt"`
			SubmittedAt       *time.Time   `json:"submittedAt"`
			AuthorAssociation string       `json:"authorAssociation"`
			Author            graphQLActor `json:"author"`
		} `json:"nodes"`
	} `json:"reviews"`

	ReviewThreads struct {
		Nodes []struct {
			Comments struct {
				Nodes []struct {
					CreatedAt         time.Time    `json:"createdAt"`
					Author            graphQLActor `json:"author"`
					ID                string       `json:"id"`
					Body              string       `json:"body"`
					Outdated          bool         `json:"outdated"`
					AuthorAssociation string       `json:"authorAssociation"`
				} `json:"nodes"`
			} `json:"comments"`
			IsResolved bool `json:"isResolved"`
			IsOutdated bool `json:"isOutdated"`
		} `json:"nodes"`
	} `json:"reviewThreads"`

	Comments struct {
		PageInfo graphQLPageInfo `json:"pageInfo"`
		Nodes    []struct {
			ID                string       `json:"id"`
			Body              string       `json:"body"`
			CreatedAt         time.Time    `json:"createdAt"`
			AuthorAssociation string       `json:"authorAssociation"`
			Author            graphQLActor `json:"author"`
		} `json:"nodes"`
	} `json:"comments"`

	TimelineItems struct {
		PageInfo graphQLPageInfo  `json:"pageInfo"`
		Nodes    []map[string]any `json:"nodes"`
	} `json:"timelineItems"`
}

// graphQLActor represents any GitHub actor (User, Bot, Organization).
type graphQLActor struct {
	Login string `json:"login"`
	ID    string `json:"id,omitempty"`
	Type  string `json:"type,omitempty"`
}

// isBot determines if an actor is a bot.
func isBot(actor graphQLActor) bool {
	if actor.Login == "" {
		return false
	}

	// Check the Type field first - most reliable signal from GitHub API
	if actor.Type == "Bot" {
		return true
	}

	// Check for bot patterns in login
	login := actor.Login
	lowerLogin := strings.ToLower(login)

	if strings.HasSuffix(login, "[bot]") ||
		strings.HasSuffix(lowerLogin, "-bot") ||
		strings.HasSuffix(lowerLogin, "_bot") ||
		strings.HasSuffix(lowerLogin, "-robot") ||
		strings.HasPrefix(lowerLogin, "bot-") {
		return true
	}

	// Many bots end with "bot" without separator (e.g., "dependabot", "renovatebot")
	if strings.HasSuffix(lowerLogin, "bot") && len(login) > 3 {
		return true
	}

	// Bot IDs typically start with "BOT_" or contain "Bot"
	return strings.HasPrefix(actor.ID, "BOT_") || strings.Contains(actor.ID, "Bot")
}

// graphQLStatusCheckNode can be either CheckRun or StatusContext.
type graphQLStatusCheckNode struct {
	StartedAt   *time.Time    `json:"startedAt,omitempty"`
	CompletedAt *time.Time    `json:"completedAt,omitempty"`
	CreatedAt   *time.Time    `json:"createdAt,omitempty"`
	Creator     *graphQLActor `json:"creator,omitempty"`
	App         *struct {
		Name       string `json:"name"`
		DatabaseID int    `json:"databaseId"`
	} `json:"app,omitempty"`
	TypeName    string `json:"__typename"`
	Name        string `json:"name,omitempty"`
	Status      string `json:"status,omitempty"`
	Conclusion  string `json:"conclusion,omitempty"`
	DetailsURL  string `json:"detailsUrl,omitempty"`
	Title       string `json:"title,omitempty"`
	Text        string `json:"text,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Context     string `json:"context,omitempty"`
	State       string `json:"state,omitempty"`
	Description string `json:"description,omitempty"`
	TargetURL   string `json:"targetUrl,omitempty"`
	DatabaseID  int    `json:"databaseId,omitempty"`
}

// graphQLPageInfo for pagination.
type graphQLPageInfo struct {
	EndCursor   string `json:"endCursor"`
	HasNextPage bool   `json:"hasNextPage"`
}
