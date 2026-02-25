package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsEncodingCancelOrTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"context.Canceled", context.Canceled, true},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"wrapped Canceled", fmt.Errorf("batch encode: %w", context.Canceled), true},
		{"wrapped DeadlineExceeded", fmt.Errorf("batch encode: %w", context.DeadlineExceeded), true},
		{"other error", errors.New("other"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEncodingCancelOrTimeout(tt.err)
			if got != tt.want {
				t.Errorf("isEncodingCancelOrTimeout(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
