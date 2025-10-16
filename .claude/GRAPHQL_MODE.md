# GraphQL Mode (Experimental)

## Overview

The prx library now includes an experimental hybrid GraphQL+REST mode that significantly reduces GitHub API calls from 13+ to approximately 3-4 calls per pull request. This hybrid approach uses GraphQL for most data and selective REST calls for missing pieces (primarily check runs and rulesets), maintaining data completeness while optimizing API usage.

This is especially useful when working with repositories that have many pull requests or when you're close to GitHub's API rate limits.

## How to Enable

Add the `WithGraphQL()` option when creating a client:

```go
import "github.com/codeGROOVE-dev/prx/pkg/prx"

// REST mode (default)
client := prx.NewClient(token)

// GraphQL mode (experimental)
client := prx.NewClient(token, prx.WithGraphQL())

// GraphQL with other options
client := prx.NewClient(token,
    prx.WithGraphQL(),
    prx.WithNoCache(),
    prx.WithLogger(customLogger),
)
```

## API Call Comparison

### REST Mode (Default)
- **Total API Calls**: 13+ per pull request
- Synchronous calls (5-6):
  - Pull request details
  - Combined status
  - Branch protection
  - Branch protection fallback
  - Repository rulesets
  - Workflow checks (if blocked)
- Parallel calls (7):
  - Commits
  - Comments
  - Reviews
  - Review comments
  - Timeline events
  - Status checks
  - Check runs

### Hybrid GraphQL+REST Mode
- **Total API Calls**: 3-4 per pull request
- Main GraphQL query (1 call) fetches:
  - Pull request details
  - All commits, comments, reviews
  - Timeline events (including draft/ready events)
  - Branch protection rules
  - Required status checks
- Additional REST calls (2-3):
  - Check runs (GraphQL's statusCheckRollup is often null)
  - Repository rulesets (not available in GraphQL)
  - Permission verification for MEMBER users (if needed)

## Benefits

- **70-75% Reduction in API Calls**: From 13+ to 3-4 calls
- **Data Completeness**: Hybrid approach maintains full data parity with REST
- **Faster Performance**: Bulk data fetching via GraphQL
- **Better Rate Limit Management**: Reduced REST API usage
- **Reliability**: REST fallbacks for critical missing data

## Testing & Comparison

You can easily compare REST vs GraphQL output using Unix diff:

```bash
# Fetch with REST (default)
prx https://github.com/owner/repo/pull/123 > rest.json

# Fetch with GraphQL
PRX_USE_GRAPHQL=1 prx https://github.com/owner/repo/pull/123 > graphql.json

# Compare outputs
diff rest.json graphql.json
```

## Known Differences

While we strive for parity, there are some minor differences:

1. **Bot Detection**: GraphQL uses login pattern matching (`-bot`, `[bot]`, `-robot` suffixes) rather than the `type` field
2. **Permissions**: MEMBER association requires an additional REST call for exact permission level
3. **Event Ordering**: Events may be ordered slightly differently but all data is present

## Limitations

- Repository rulesets are not available via GraphQL (requires 1 REST call)
- Some edge cases in bot detection may differ
- Write access detection for MEMBER users is approximated
- **Check Runs**: GraphQL has significant limitations fetching check runs:
  - `statusCheckRollup` is often null for many repositories
  - Check runs are not included in timeline events
  - Fork PRs may have null `headRef`, preventing status checks from being fetched
  - Draft PRs may have incomplete status information
  - REST API fetches check runs separately and more reliably
- **Old PRs**: Check runs from old PRs (>90 days) may not be available via GraphQL
  - GitHub may not expose historical check run data consistently
  - REST API may have cached or different retention policies
  - For critical historical analysis, REST mode may be more reliable
- **Event Completeness**: GraphQL may miss certain events:
  - Check run events are often missing entirely
  - Some timeline event types may not be captured

## Stability

This is an **experimental feature**. While it has been tested, you may encounter:
- Minor data differences compared to REST
- Edge cases not yet handled
- Need to fall back to REST for certain operations

Please report any issues or discrepancies you find.

## Environment Variable

You can also enable GraphQL mode via environment variable:

```bash
export PRX_USE_GRAPHQL=1
prx https://github.com/owner/repo/pull/123
```

## Metrics

When GraphQL mode is enabled, the client logs the mode and number of API calls:

```
INFO GraphQL mode enabled owner=golang repo=go pr=123
INFO successfully fetched pull request via GraphQL api_calls_made="2 (vs 13+ with REST)"
```

## Future Improvements

- Full GraphQL implementation without REST fallbacks
- Better caching integration
- Pagination support for very large PRs
- WebSocket subscriptions for real-time updates