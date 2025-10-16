# GitHub API Optimization Guide

## Current API Call Analysis

The current implementation makes **13+ REST API calls** per pull request fetch:

### Synchronous Calls (Required Checks Detection)
1. GraphQL for refUpdateRule
2. Combined status (`/commits/{sha}/status`)
3. Branch protection (`/branches/{branch}/protection`)
4. Branch protection fallback (`/branches/{branch}/protection/required_status_checks`)
5. Repository rulesets (`/rulesets`)
6. Workflows (if PR blocked) - multiple calls

### Parallel Calls (Event Fetching)
7. Commits (`/pulls/{pr}/commits`)
8. Comments (`/issues/{pr}/comments`)
9. Reviews (`/pulls/{pr}/reviews`)
10. Review comments (`/pulls/{pr}/comments`)
11. Timeline (`/issues/{pr}/timeline`)
12. Status checks (`/statuses/{sha}`)
13. Check runs (`/commits/{sha}/check-runs`)

## Optimization Opportunities

### 1. **Single GraphQL Query Solution** (RECOMMENDED)
Replace all 13+ REST calls with **1 GraphQL query** that fetches everything:
- **API Quota Savings**: ~92% reduction (1 call vs 13+)
- **Performance**: Faster due to single round-trip
- **Implementation**: See `fetchers_graphql_optimized.go`

### 2. **Remove Redundant Calls**
Several calls appear redundant or could be eliminated:

#### a. Combined Status API
- **Current**: Fetches `/commits/{sha}/status`
- **Issue**: Only used for logging, not for actual required checks
- **Fix**: Remove this call entirely

#### b. Branch Protection Fallback
- **Current**: Tries main endpoint, then fallback
- **Issue**: Two calls when one would suffice
- **Fix**: Use only the main endpoint or handle in GraphQL

#### c. Workflow Checks
- **Current**: Multiple calls to workflows API when PR is blocked
- **Issue**: Expensive and often unnecessary
- **Fix**: Remove or make optional

### 3. **Batch Timeline Events**
The timeline API already includes many event types. It could potentially replace:
- Comments
- Some review events
- Assignment/label events

### 4. **Cache Optimization**
Improve caching to reduce redundant API calls:
- Cache required checks for longer (they rarely change)
- Cache user permissions
- Use ETags for conditional requests

## Implementation Priority

### High Priority (Quick Wins)
1. **Remove combined status call** - Easy, saves 1 API call
2. **Remove branch protection fallback** - Easy, saves 1 API call
3. **Skip workflow checks unless explicitly needed** - Easy, saves 2-5 API calls

### Medium Priority (More Complex)
1. **Implement single GraphQL query** - Best long-term solution
2. **Use timeline API to replace multiple event calls** - Moderate complexity

### Low Priority (Nice to Have)
1. **Implement ETag caching** - Complex but useful for high-traffic repos
2. **Add permission caching across requests** - Helps with repeated user checks

## GraphQL Query Benefits

The provided GraphQL implementation (`fetchers_graphql_optimized.go`) offers:

1. **Single API Call**: All data in one request
2. **Precise Data Selection**: Only fetch needed fields
3. **Rate Limit Info**: Built-in rate limit tracking
4. **Future-Proof**: Easy to add new fields without new endpoints

## Migration Path

1. **Phase 1**: Remove redundant REST calls (combined status, fallbacks)
2. **Phase 2**: Add GraphQL query as optional alternative
3. **Phase 3**: Migrate to GraphQL by default with REST fallback
4. **Phase 4**: Remove REST implementation

## Expected Savings

- **Current**: 13+ API calls per PR fetch
- **After Quick Wins**: 8-10 API calls (38% reduction)
- **With GraphQL**: 1 API call (92% reduction)

For a user checking 100 PRs:
- **Current**: 1,300+ API calls (likely hitting rate limits)
- **With GraphQL**: 100 API calls (well within limits)