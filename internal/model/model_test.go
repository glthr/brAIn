package model

import (
	"testing"
)

func TestEncodeDecodeTags(t *testing.T) {
	tests := []struct {
		name string
		tags []string
	}{
		{"nil", nil},
		{"empty", []string{}},
		{"single", []string{"go"}},
		{"multiple", []string{"go", "testing", "concurrency"}},
		{"with spaces", []string{"my tag", "another tag"}},
		{"with special chars", []string{`"quoted"`, "a,b", "x:y"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeTags(tt.tags)
			decoded := DecodeTags(encoded)

			if len(tt.tags) == 0 {
				if encoded != "[]" {
					t.Errorf("EncodeTags(%v) = %q, want %q", tt.tags, encoded, "[]")
				}
				if decoded != nil {
					t.Errorf("DecodeTags(%q) = %v, want nil", encoded, decoded)
				}
				return
			}

			if len(decoded) != len(tt.tags) {
				t.Fatalf("round-trip length: got %d, want %d", len(decoded), len(tt.tags))
			}
			for i := range tt.tags {
				if decoded[i] != tt.tags[i] {
					t.Errorf("round-trip [%d]: got %q, want %q", i, decoded[i], tt.tags[i])
				}
			}
		})
	}
}

func TestDecodeTagsEdgeCases(t *testing.T) {
	if got := DecodeTags(""); got != nil {
		t.Errorf("DecodeTags(\"\") = %v, want nil", got)
	}
	if got := DecodeTags("[]"); got != nil {
		t.Errorf("DecodeTags(\"[]\") = %v, want nil", got)
	}
	if got := DecodeTags("not json"); got != nil {
		t.Errorf("DecodeTags(\"not json\") = %v, want nil", got)
	}
}

func TestEncodeDecodeMetadata(t *testing.T) {
	tests := []struct {
		name string
		meta map[string]string
	}{
		{"nil", nil},
		{"empty", map[string]string{}},
		{"single", map[string]string{"key": "value"}},
		{"multiple", map[string]string{"a": "1", "b": "2", "c": "3"}},
		{"special chars", map[string]string{"k": `"quoted"`, "x": "a,b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeMetadata(tt.meta)
			decoded := DecodeMetadata(encoded)

			if len(tt.meta) == 0 {
				if encoded != "{}" {
					t.Errorf("EncodeMetadata(%v) = %q, want %q", tt.meta, encoded, "{}")
				}
				if decoded != nil {
					t.Errorf("DecodeMetadata(%q) = %v, want nil", encoded, decoded)
				}
				return
			}

			if len(decoded) != len(tt.meta) {
				t.Fatalf("round-trip length: got %d, want %d", len(decoded), len(tt.meta))
			}
			for k, v := range tt.meta {
				if decoded[k] != v {
					t.Errorf("round-trip [%s]: got %q, want %q", k, decoded[k], v)
				}
			}
		})
	}
}

func TestDecodeMetadataEdgeCases(t *testing.T) {
	if got := DecodeMetadata(""); got != nil {
		t.Errorf("DecodeMetadata(\"\") = %v, want nil", got)
	}
	if got := DecodeMetadata("{}"); got != nil {
		t.Errorf("DecodeMetadata(\"{}\") = %v, want nil", got)
	}
	if got := DecodeMetadata("not json"); got != nil {
		t.Errorf("DecodeMetadata(\"not json\") = %v, want nil", got)
	}
}
