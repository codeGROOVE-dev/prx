package prx

// completeGraphQLQuery is the GraphQL query that fetches all PR data.
// This replaces 13+ REST API calls with a single comprehensive query.
const completeGraphQLQuery = `
query($owner: String!, $repo: String!, $number: Int!, $prCursor: String, $reviewCursor: String, $timelineCursor: String, $commentCursor: String) {
	repository(owner: $owner, name: $repo) {
		pullRequest(number: $number) {
			id
			number
			title
			body
			state
			createdAt
			updatedAt
			closedAt
			mergedAt
			isDraft
			additions
			deletions
			changedFiles
			mergeable
			mergeStateStatus
			authorAssociation

			author {
				__typename
				login
				... on User {
					id
				}
				... on Bot {
					id
				}
			}

			mergedBy {
				__typename
				login
				... on User {
					id
				}
				... on Bot {
					id
				}
			}

			assignees(first: 100) {
				nodes {
					login
					... on User {
						id
					}
				}
			}

			labels(first: 100) {
				nodes {
					name
				}
			}

			participants(first: 100) {
				nodes {
					login
					... on User {
						id
					}
				}
			}

			reviewRequests(first: 100) {
				nodes {
					requestedReviewer {
						... on User {
							login
							id
						}
						... on Team {
							name
							id
						}
					}
				}
			}

			baseRef {
				name
				target {
					... on Commit {
						oid
					}
				}
				refUpdateRule {
					requiredStatusCheckContexts
				}
				branchProtectionRule {
					requiredStatusCheckContexts
					requiresStatusChecks
					requiredApprovingReviewCount
					requiresApprovingReviews
				}
			}

			headRef {
				name
				target {
					... on Commit {
						oid
						statusCheckRollup {
							state
							contexts(first: 100) {
								nodes {
									__typename
									... on CheckRun {
										name
										status
										conclusion
										startedAt
										completedAt
										detailsUrl
										title: title
										text: text
										summary: summary
										databaseId
									}
									... on StatusContext {
										context
										state
										description
										targetUrl
										createdAt
										creator {
											__typename
											login
											... on User {
												id
											}
											... on Bot {
												id
											}
										}
									}
								}
							}
						}
					}
				}
			}

			commits(first: 100, after: $prCursor) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					commit {
						oid
						message
						committedDate
						author {
							name
							email
							user {
								login
								... on User {
									id
								}
							}
						}
					}
				}
			}

			reviews(first: 100, after: $reviewCursor) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					id
					state
					body
					createdAt
					submittedAt
					authorAssociation
					author {
						__typename
						login
						... on User {
							id
						}
						... on Bot {
							id
						}
					}
				}
			}

			reviewThreads(first: 100) {
				nodes {
					isResolved
					isOutdated
					comments(first: 100) {
						nodes {
							id
							body
							createdAt
							outdated
							authorAssociation
							author {
								__typename
								login
								... on User {
									id
								}
								... on Bot {
									id
								}
							}
						}
					}
				}
			}

			comments(first: 100, after: $commentCursor) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					id
					body
					createdAt
					authorAssociation
					author {
						__typename
						login
						... on User {
							id
						}
						... on Bot {
							id
						}
					}
				}
			}

			timelineItems(first: 100, after: $timelineCursor) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					__typename
					... on AssignedEvent {
						id
						createdAt
						actor {
							__typename
							login
							... on User {
								id
							}
							... on Bot {
								id
							}
						}
						assignee {
							... on User {
								login
								id
							}
							... on Bot {
								login
								id
							}
						}
					}
					... on UnassignedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
						assignee {
							... on User {
								login
								id
							}
						}
					}
					... on LabeledEvent {
						id
						createdAt
						label {
							name
						}
						actor {
							__typename
							login
						}
					}
					... on UnlabeledEvent {
						id
						createdAt
						label {
							name
						}
						actor {
							__typename
							login
						}
					}
					... on MilestonedEvent {
						id
						createdAt
						milestoneTitle
						actor {
							__typename
							login
						}
					}
					... on DemilestonedEvent {
						id
						createdAt
						milestoneTitle
						actor {
							__typename
							login
						}
					}
					... on ReviewRequestedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
						requestedReviewer {
							... on User {
								login
								id
							}
							... on Team {
								name
								id
							}
							... on Bot {
								login
								id
							}
						}
					}
					... on ReviewRequestRemovedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
						requestedReviewer {
							... on User {
								login
							}
							... on Team {
								name
							}
						}
					}
					... on ClosedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on ReopenedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on MergedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on MentionedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on ReadyForReviewEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on ConvertToDraftEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on AutoMergeEnabledEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on AutoMergeDisabledEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on ReviewDismissedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
						dismissalMessage
					}
					... on HeadRefDeletedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on RenamedTitleEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
						previousTitle
						currentTitle
					}
					... on BaseRefChangedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on BaseRefForcePushedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on HeadRefForcePushedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on HeadRefRestoredEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on LockedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on UnlockedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on AddedToMergeQueueEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on RemovedFromMergeQueueEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on AutomaticBaseChangeSucceededEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on AutomaticBaseChangeFailedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on ConnectedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on DisconnectedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on CrossReferencedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on ReferencedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on SubscribedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on UnsubscribedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on DeployedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on DeploymentEnvironmentChangedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on PinnedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on UnpinnedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on TransferredEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
					... on UserBlockedEvent {
						id
						createdAt
						actor {
							__typename
							login
						}
					}
				}
			}
		}
	}

	rateLimit {
		cost
		remaining
		resetAt
		limit
	}
}`
