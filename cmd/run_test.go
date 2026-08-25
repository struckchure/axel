package main

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/struckchure/axel/core/runner"
)

func TestFormatUUIDs(t *testing.T) {
	uuidBytes := [16]byte{138, 186, 24, 167, 17, 231, 75, 36, 150, 150, 124, 63, 83, 204, 202, 73}
	expectedUUIDStr := "8aba18a7-11e7-4b24-9696-7c3f53ccca49"

	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{
			name:     "nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "raw [16]byte uuid",
			input:    uuidBytes,
			expected: expectedUUIDStr,
		},
		{
			name:     "pointer *[16]byte uuid",
			input:    &uuidBytes,
			expected: expectedUUIDStr,
		},
		{
			name:     "raw []byte uuid of length 16",
			input:    uuidBytes[:],
			expected: expectedUUIDStr,
		},
		{
			name: "pgtype.UUID valid",
			input: pgtype.UUID{
				Bytes: uuidBytes,
				Valid: true,
			},
			expected: expectedUUIDStr,
		},
		{
			name: "pgtype.UUID invalid",
			input: pgtype.UUID{
				Bytes: uuidBytes,
				Valid: false,
			},
			expected: nil,
		},
		{
			name: "pointer *pgtype.UUID valid",
			input: &pgtype.UUID{
				Bytes: uuidBytes,
				Valid: true,
			},
			expected: expectedUUIDStr,
		},
		{
			name:     "string remains unchanged",
			input:    "hello",
			expected: "hello",
		},
		{
			name: "map[string]any with uuid",
			input: map[string]any{
				"id":   uuidBytes,
				"name": "Alice",
			},
			expected: map[string]any{
				"id":   expectedUUIDStr,
				"name": "Alice",
			},
		},
		{
			name: "runner.Row with uuid",
			input: runner.Row{
				"id":   uuidBytes,
				"name": "Bob",
			},
			expected: runner.Row{
				"id":   expectedUUIDStr,
				"name": "Bob",
			},
		},
		{
			name: "slice of any with uuid",
			input: []any{
				uuidBytes,
				"some-other-val",
			},
			expected: []any{
				expectedUUIDStr,
				"some-other-val",
			},
		},
		{
			name: "nested map and slice structure",
			input: map[string]any{
				"user": map[string]any{
					"id": uuidBytes,
				},
				"tags": []any{
					map[string]any{"id": uuidBytes},
				},
			},
			expected: map[string]any{
				"user": map[string]any{
					"id": expectedUUIDStr,
				},
				"tags": []any{
					map[string]any{"id": expectedUUIDStr},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatUUIDs(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("formatUUIDs() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
