# Phase 4: Keep unchanged pending-review output stable

Back to [[plans/123-runtime-output-idempotence/overview]]

## Goal
Keep unchanged pending-review output stable.

## Changes
- loop/pending_review.go — opt in at the persistence boundary.
- loop/pending_review_idle_test.go — test stable and changed review snapshots.

## Data structures
canonical PendingReview map and sorted pendingReviewSnapshot.

## Routing and principles
Implementation: Codex, current configured model; independent review: Claude CLI. Apply [[principles/fix-root-causes]], [[principles/make-operations-idempotent]] and [[principles/prove-it-works]].

## Verification
Static: gofmt, scoped tests and vet; full pnpm check at integrated verification. Runtime: Repeated empty/populated review state preserves identity/mtime; a real review addition/removal changes output; missing/corrupt files repair; existing flush barrier failures still propagate.
