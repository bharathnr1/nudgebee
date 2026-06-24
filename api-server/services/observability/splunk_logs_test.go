package observability

import (
	"errors"
	"testing"

	"nudgebee/services/integrations"
	"nudgebee/services/security"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	s := &SplunkLogSource{
		GetConfigs: okConfigs,
		LogSearch: func(integrations.SplunkO11yConnConfig, string, int64, int64, int) ([]integrations.O11yLogEntry, error) {
			return []integrations.O11yLogEntry{
				{Attributes: map[string]any{"message": "a", "service.version": "1.2.3"}},
				{Attributes: map[string]any{"message": "b", "http.status_code": 500, "service.version": "1.2.3"}},
			}, nil
		},
	}
	labels, err := s.QueryLabels(mockRequestContext(), FetchLogLabelRequest{AccountId: "acct-1"})
	require.NoError(t, err)
	names := labelNames(labels)
	assert.Equal(t, []string{"http.status_code", "message", "service.version"}, names)
	assert.Contains(t, names, "service.version", "custom OTel attribute must be discovered dynamically")
}

func TestSplunkQueryLabels_FallbackOnEmptySample(t *testing.T) {
	s := &SplunkLogSource{
		GetConfigs: okConfigs,
		LogSearch: func(integrations.SplunkO11yConnConfig, string, int64, int64, int) ([]integrations.O11yLogEntry, error) {
			return nil, nil // no entries sampled
		},
	}
	labels, err := s.QueryLabels(mockRequestContext(), FetchLogLabelRequest{AccountId: "acct-1"})
	require.NoError(t, err)
	assert.Equal(t, splunkO11yFallbackLogLabelNames, labelNames(labels),
		"empty sample must fall back to the static field set, not return empty")
}

func TestSplunkQueryLabels_FallbackOnSearchError(t *testing.T) {
	s := &SplunkLogSource{
		GetConfigs: okConfigs,
		LogSearch: func(integrations.SplunkO11yConnConfig, string, int64, int64, int) ([]integrations.O11yLogEntry, error) {
			return nil, errors.New("splunk unreachable")
		},
	}
	labels, err := s.QueryLabels(mockRequestContext(), FetchLogLabelRequest{AccountId: "acct-1"})
	require.NoError(t, err, "a sample-query failure must not error out the label list")
	assert.Equal(t, splunkO11yFallbackLogLabelNames, labelNames(labels))
}

func TestSplunkQueryLabels_FallbackOnConfigError(t *testing.T) {
	s := &SplunkLogSource{
		GetConfigs: func(*security.RequestContext, string) (integrations.SplunkO11yConnConfig, error) {
			return integrations.SplunkO11yConnConfig{}, errors.New("no config")
		},
		LogSearch: func(integrations.SplunkO11yConnConfig, string, int64, int64, int) ([]integrations.O11yLogEntry, error) {
			t.Fatal("log search must not be called when config lookup fails")
			return nil, nil
		},
	}
	labels, err := s.QueryLabels(mockRequestContext(), FetchLogLabelRequest{AccountId: "acct-1"})
	require.NoError(t, err)
	assert.Equal(t, splunkO11yFallbackLogLabelNames, labelNames(labels))
}

func TestDedupeO11yFieldLabels_DistinctSorted(t *testing.T) {
	entries := []integrations.O11yLogEntry{
		{Attributes: map[string]any{"b": 1, "a": 2}},
		{Attributes: map[string]any{"a": 3, "c": 4, "": "skip-empty-key"}},
	}
	assert.Equal(t, []string{"a", "b", "c"}, labelNames(dedupeO11yFieldLabels(entries)))
}
