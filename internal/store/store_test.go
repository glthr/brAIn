package store

import (
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		input string
		want  time.Time
	}{
		{"2025-01-15 10:30:00", time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)},
		{"2025-01-15 10:30:00.123456", time.Date(2025, 1, 15, 10, 30, 0, 123456000, time.UTC)},
		{"2025-01-15T10:30:00Z", time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)},
		{"2025-01-15T10:30:00+00:00", time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)},
		{"2025-01-15 10:30:00+00:00", time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)},
		{"garbage", time.Time{}},
		{"", time.Time{}},
	}

	for _, tt := range tests {
		got := ParseTime(tt.input)
		if !got.Equal(tt.want) {
			t.Errorf("ParseTime(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
