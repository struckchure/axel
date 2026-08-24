package runner

import (
	"reflect"
	"testing"
)

func TestParseParams(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []string
		expected map[string]any
		wantErr  bool
	}{
		{
			name:   "strict json",
			inputs: []string{`{"skip": 1, "limit": 20}`},
			expected: map[string]any{
				"skip":  float64(1),
				"limit": float64(20),
			},
		},
		{
			name:   "relaxed json with unquoted keys and single quotes",
			inputs: []string{`{skip: 1, limit: 20, status: 'active'}`},
			expected: map[string]any{
				"skip":   float64(1),
				"limit":  float64(20),
				"status": "active",
			},
		},
		{
			name:   "prefixed params= relaxed json",
			inputs: []string{`params={skip: 1, limit: 20}`},
			expected: map[string]any{
				"skip":  float64(1),
				"limit": float64(20),
			},
		},
		{
			name:   "prefixed params= strict json",
			inputs: []string{`params={"skip": 10, "limit": 50}`},
			expected: map[string]any{
				"skip":  float64(10),
				"limit": float64(50),
			},
		},
		{
			name:   "key value pairs",
			inputs: []string{"skip=1", "limit=20", "active=true", "name=Alice"},
			expected: map[string]any{
				"skip":   int64(1),
				"limit":  int64(20),
				"active": true,
				"name":   "Alice",
			},
		},
		{
			name:   "multi array param in json",
			inputs: []string{`{"conditions": ["Hot", "Cold", "Fragile"]}`},
			expected: map[string]any{
				"conditions": []any{"Hot", "Cold", "Fragile"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseParams(tc.inputs...)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("got %#v, expected %#v", got, tc.expected)
			}
		})
	}
}
