package triage

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
)

// TestDispatchRuleMatchCountUpdates verifies the bounded, non-blocking match-count
// dispatch that replaced the previous unbounded `go updateRuleMatchCount` per match
// (#286). It substitutes the DB-backed updater with a fake and isolates the package
// semaphore per subtest, so it's deterministic and needs no database.
func TestDispatchRuleMatchCountUpdates(t *testing.T) {
	origUpdater := ruleMatchCountUpdate
	origSem := ruleMatchUpdateSem
	t.Cleanup(func() {
		ruleMatchCountUpdate = origUpdater
		ruleMatchUpdateSem = origSem
	})

	t.Run("dispatches every winning rule id exactly once", func(t *testing.T) {
		ruleMatchUpdateSem = make(chan struct{}, ruleMatchUpdateMaxConcurrency) // empty
		ids := []string{"r1", "r2", "r3"}

		var mu sync.Mutex
		got := map[string]int{}
		var wg sync.WaitGroup
		wg.Add(len(ids))
		ruleMatchCountUpdate = func(_ context.Context, _ *sqlx.DB, id string) {
			mu.Lock()
			got[id]++
			mu.Unlock()
			wg.Done()
		}

		dispatchRuleMatchCountUpdates(context.Background(), nil, ids)
		wg.Wait() // deterministic: the fake signals once per id

		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, map[string]int{"r1": 1, "r2": 1, "r3": 1}, got)
	})

	t.Run("empty rule ids is a no-op", func(t *testing.T) {
		ruleMatchUpdateSem = make(chan struct{}, ruleMatchUpdateMaxConcurrency)
		var called int32
		ruleMatchCountUpdate = func(_ context.Context, _ *sqlx.DB, _ string) { atomic.AddInt32(&called, 1) }

		dispatchRuleMatchCountUpdates(context.Background(), nil, nil)
		assert.Equal(t, int32(0), atomic.LoadInt32(&called))
	})

	t.Run("drops update when concurrency is saturated", func(t *testing.T) {
		// Pre-fill a fresh semaphore to capacity so dispatch takes the drop branch.
		full := make(chan struct{}, ruleMatchUpdateMaxConcurrency)
		for i := 0; i < ruleMatchUpdateMaxConcurrency; i++ {
			full <- struct{}{}
		}
		ruleMatchUpdateSem = full

		var called int32
		ruleMatchCountUpdate = func(_ context.Context, _ *sqlx.DB, _ string) { atomic.AddInt32(&called, 1) }

		dispatchRuleMatchCountUpdates(context.Background(), nil, []string{"r1"})
		// Deterministic: a full semaphore forces the default branch — no goroutine
		// is spawned, so the updater is never invoked.
		assert.Equal(t, int32(0), atomic.LoadInt32(&called),
			"match-count update must be dropped (not spawned) when the bound is saturated")
	})
}
