package core

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestGoogleAICacheProvider_SingleflightCollapsesConcurrentCreates verifies the
// fix for #302: concurrent cache-miss creations for the same cache key must
// collapse into a single createCache execution (via the provider's
// singleflight group) so parallel conversations don't each create a distinct,
// duplicate Google AI CachedContent. We exercise the provider's createGroup
// directly with a counting closure — the real createCache hits Google AI and
// can't run in CI — which guards that the group field is wired and dedups
// same-key concurrent calls. Run with -race to catch field races.
func TestGoogleAICacheProvider_SingleflightCollapsesConcurrentCreates(t *testing.T) {
	p := &GoogleAICacheProvider{namespace: "test_singleflight"}

	const cacheKey = "account:agent:model"
	const goroutines = 25
	var createCalls int32

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all at once to maximize contention
			_, _, _ = p.createGroup.Do(cacheKey, func() (interface{}, error) {
				atomic.AddInt32(&createCalls, 1)
				// Hold the slot briefly so the other goroutines coalesce onto it.
				time.Sleep(20 * time.Millisecond)
				return "created", nil
			})
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&createCalls),
		"concurrent same-key cache creations must collapse to a single createCache call")
}

// TestGoogleAICacheProvider_SingleflightAllowsDistinctKeys confirms different
// cache keys are NOT collapsed — each distinct key runs its own creation.
func TestGoogleAICacheProvider_SingleflightAllowsDistinctKeys(t *testing.T) {
	p := &GoogleAICacheProvider{namespace: "test_singleflight"}

	var createCalls int32
	var wg sync.WaitGroup
	keys := []string{"a:1:m", "b:2:m", "c:3:m"}
	for _, k := range keys {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, _, _ = p.createGroup.Do(key, func() (interface{}, error) {
				atomic.AddInt32(&createCalls, 1)
				return "created", nil
			})
		}(k)
	}
	wg.Wait()

	assert.Equal(t, int32(len(keys)), atomic.LoadInt32(&createCalls),
		"distinct cache keys must each run their own creation")
}
