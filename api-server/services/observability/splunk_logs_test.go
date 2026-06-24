package observability

import (
	"errors"
	"testing"

	"nudgebee/services/integrations"
	"nudgebee/services/security"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withSplunkO11ySeams swaps the integrations seams for the duration of fn and
// restores them after, so QueryLabels can be exercised without a live backend.
func withSplunkO11ySeams(
	t *testing.T,
	getConfigs func(*security.RequestContext, string) (integrations.SplunkO11yConnConfig, error),
	logSearch func(integrations.SplunkO11yConnConfig, string, int64, int64, int) ([]integrations.O11yLogEntry, error),
	fn func(),
) {
	t.Helper()
	origGet, origSearch := splunkO11yGetConfigs, splunkO11yLogSearch
	splunkO11yGetConfigs = getConfigs
	splunkO11yLogSearch = logSearch
	t.Cleanup(func() {
		splunkO11yGetConfigs = origGet
		splunkO11yLogSearch = origSearch
	})
	fn()
}

func labelNames(labels []OutputLogLabel) []string {
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = l.Label
	}
	return out
}

func okConfigs(*security.RequestContext, string) (integrations.SplunkO11yConnConfig, error) {
	return integrations.SplunkO11yConnConfig{}, nil
}

func TestSplunkQueryLabels_DynamicDiscovery(t *testing.T) {
	// A custom OTel attribute present in the sampled logs must surface, and the
	// output must be the sorted set of distinct attribute keys.
	search := func(integrations.SplunkO11yConnConfig, string, int64, int64, int) ([]integrations.O11yLogEntry, error) {
		return []integrations.O11yLogEntry{
			{Attributes: map[string]any{"message": "a", "service.version": "1.2.3"}},
			{Attributes: map[string]any{"message": "b", "http.status_code": 500, "service.version": "1.2.3"}},
		}, nil
	}
	withSplunkO11ySeams(t, okConfigs, search, func() {
		s := &SplunkLogSource{}
		labels, err := s.QueryLabels(mockRequestContext(), FetchLogLabelRequest{AccountId: "acct-1"})
		require.NoError(t, err)
		names := labelNames(labels)
		// Distinct + sorted.
		assert.Equal(t, []string{"http.status_code", "message", "service.version"}, names)
		assert.Contains(t, names, "service.version", "custom OTel attribute must be discovered dynamically")
	})
}

func TestSplunkQueryLabels_FallbackOnEmptySample(t *testing.T) {
	search := func(integrations.SplunkO11yConnConfig, string, int64, int64, int) ([]integrations.O11yLogEntry, error) {
		return nil, nil // no entries sampled
	}
	withSplunkO11ySeams(t, okConfigs, search, func() {
		s := &SplunkLogSource{}
		labels, err := s.QueryLabels(mockRequestContext(), FetchLogLabelRequest{AccountId: "acct-1"})
		require.NoError(t, err)
		assert.Equal(t, splunkO11yFallbackLogLabelNames, labelNames(labels),
			"empty sample must fall back to the static field set, not return empty")
	})
}

func TestSplunkQueryLabels_FallbackOnSearchError(t *testing.T) {
	search := func(integrations.SplunkO11yConnConfig, string, int64, int64, int) ([]integrations.O11yLogEntry, error) {
		return nil, errors.New("splunk unreachable")
	}
	withSplunkO11ySeams(t, okConfigs, search, func() {
		s := &SplunkLogSource{}
		labels, err := s.QueryLabels(mockRequestContext(), FetchLogLabelRequest{AccountId: "acct-1"})
		require.NoError(t, err, "a sample-query failure must not error out the label list")
		assert.Equal(t, splunkO11yFallbackLogLabelNames, labelNames(labels))
	})
}

func TestSplunkQueryLabels_FallbackOnConfigError(t *testing.T) {
	getConfigs := func(*security.RequestContext, string) (integrations.SplunkO11yConnConfig, error) {
		return integrations.SplunkO11yConnConfig{}, errors.New("no config")
	}
	search := func(integrations.SplunkO11yConnConfig, string, int64, int64, int) ([]integrations.O11yLogEntry, error) {
		t.Fatal("log search must not be called when config lookup fails")
		return nil, nil
	}
	withSplunkO11ySeams(t, getConfigs, search, func() {
		s := &SplunkLogSource{}
		labels, err := s.QueryLabels(mockRequestContext(), FetchLogLabelRequest{AccountId: "acct-1"})
		require.NoError(t, err)
		assert.Equal(t, splunkO11yFallbackLogLabelNames, labelNames(labels))
	})
}

func TestDedupeO11yFieldLabels_DistinctSorted(t *testing.T) {
	entries := []integrations.O11yLogEntry{
		{Attributes: map[string]any{"b": 1, "a": 2}},
		{Attributes: map[string]any{"a": 3, "c": 4, "": "skip-empty-key"}},
	}
	assert.Equal(t, []string{"a", "b", "c"}, labelNames(dedupeO11yFieldLabels(entries)))
}
