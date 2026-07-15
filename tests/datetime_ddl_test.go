package tests

import (
	"strings"
	"testing"
)

// datetime columns MUST be created as TIMESTAMPTZ, not naive TIMESTAMP. A naive
// timestamp serializes without a timezone offset in row_to_json output, which
// breaks Go's encoding/json (RFC3339) when nested relations are decoded from
// JSON (see the `json_agg(row_to_json(...))` subqueries in core/compiler).
//
// This also guards the date/time/decimal entries in mapType, which previously
// fell through to UUID.
func TestDDLTimestampTypes(t *testing.T) {
	schema := `type Event {
  required id: uuid;
  created_at: datetime;
  event_day: date;
  event_time: time;
  amount: decimal;
}`

	up := genUp(t, schema)

	cases := []struct {
		col, sqlType string
	}{
		{"created_at", "TIMESTAMPTZ"},
		{"event_day", "DATE"},
		{"event_time", "TIME"},
		{"amount", "NUMERIC"},
	}
	for _, c := range cases {
		if !strings.Contains(up, c.sqlType) {
			t.Errorf("expected column %q to map to %q, but it is missing from DDL:\n%s", c.col, c.sqlType, up)
		}
	}

	// A naive TIMESTAMP (word-boundary) must never be emitted for datetime.
	for _, line := range strings.Split(up, "\n") {
		if strings.Contains(line, "created_at") && strings.Contains(line, "TIMESTAMP") && !strings.Contains(line, "TIMESTAMPTZ") {
			t.Errorf("datetime column emitted naive TIMESTAMP instead of TIMESTAMPTZ:\n%s", line)
		}
	}
}
