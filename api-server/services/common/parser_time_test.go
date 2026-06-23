package common

import (
	stdjson "encoding/json"
	"testing"
	"time"
)

func TestHasTimezoneIndicator(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "trailing Z", in: "2006-01-02T15:04:05Z", want: true},
		{name: "positive offset", in: "2006-01-02T15:04:05+07:00", want: true},
		{name: "negative offset", in: "2006-01-02T15:04:05-07:00", want: true},
		{name: "no timezone", in: "2006-01-02T15:04:05", want: false},
		{name: "date only (hyphens must not count)", in: "2006-01-02", want: false},
		{name: "empty string", in: "", want: false},
		{name: "short string under date length", in: "15:04:05", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasTimezoneIndicator(tt.in); got != tt.want {
				t.Errorf("HasTimezoneIndicator(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseUnixTimestamp(t *testing.T) {
	// 2021-01-01T00:00:00Z expressed in each unit.
	const sec = int64(1609459200)
	const milli = sec * 1000
	const nano = sec * 1_000_000_000

	tests := []struct {
		name string
		in   int64
		want time.Time
	}{
		{name: "seconds", in: sec, want: time.Unix(sec, 0)},
		{name: "milliseconds", in: milli, want: time.UnixMilli(milli)},
		{name: "nanoseconds", in: nano, want: time.Unix(0, nano)},
		{name: "zero is seconds", in: 0, want: time.Unix(0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseUnixTimestamp(tt.in); !got.Equal(tt.want) {
				t.Errorf("ParseUnixTimestamp(%d) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseTimeValue(t *testing.T) {
	want := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("time.Time passes through", func(t *testing.T) {
		got, err := ParseTimeValue(want)
		if err != nil || !got.Equal(want) {
			t.Errorf("ParseTimeValue(time.Time) = %v, %v; want %v, nil", got, err, want)
		}
	})

	t.Run("RFC3339 string", func(t *testing.T) {
		got, err := ParseTimeValue("2021-01-01T00:00:00Z")
		if err != nil || !got.Equal(want) {
			t.Errorf("got %v, %v; want %v, nil", got, err, want)
		}
	})

	t.Run("date-only string", func(t *testing.T) {
		got, err := ParseTimeValue("2021-01-01")
		if err != nil || !got.Equal(want) {
			t.Errorf("got %v, %v; want %v, nil", got, err, want)
		}
	})

	t.Run("unix-seconds string", func(t *testing.T) {
		got, err := ParseTimeValue("1609459200")
		if err != nil || !got.Equal(want) {
			t.Errorf("got %v, %v; want %v, nil", got, err, want)
		}
	})

	t.Run("numeric types", func(t *testing.T) {
		for _, v := range []any{int(1609459200), int64(1609459200), float64(1609459200)} {
			got, err := ParseTimeValue(v)
			if err != nil || !got.Equal(want) {
				t.Errorf("ParseTimeValue(%T) = %v, %v; want %v, nil", v, got, err, want)
			}
		}
	})

	t.Run("json.Number integer", func(t *testing.T) {
		got, err := ParseTimeValue(stdjson.Number("1609459200"))
		if err != nil || !got.Equal(want) {
			t.Errorf("got %v, %v; want %v, nil", got, err, want)
		}
	})

	t.Run("unparseable string errors", func(t *testing.T) {
		if _, err := ParseTimeValue("not-a-time"); err == nil {
			t.Error("expected error for unparseable string, got nil")
		}
	})

	t.Run("unsupported type errors", func(t *testing.T) {
		if _, err := ParseTimeValue([]string{"x"}); err == nil {
			t.Error("expected error for unsupported type, got nil")
		}
	})
}
