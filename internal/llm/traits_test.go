package llm

import "testing"

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no fences", `{"key": "value"}`, `{"key": "value"}`},
		{"json fences", "```json\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"plain fences", "```\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"with whitespace", "  ```json\n{\"key\": \"value\"}\n```  ", `{"key": "value"}`},
		{"empty", "", ""},
		{"only fences", "```\n```", ""},
		{"nested content", "```json\n{\"a\": \"```b```\"}\n```", `{"a": "` + "```b```" + `"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripCodeFences(tt.input)
			if got != tt.want {
				t.Errorf("stripCodeFences(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"abc", 0, "..."},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
		}
	}
}
