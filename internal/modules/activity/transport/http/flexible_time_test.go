package http

import (
	"encoding/json"
	"testing"
	"time"
)

// TestFlexibleTimeAcceptsDatetimeLocal pins the bug that made every activity save
// fail: <input type="datetime-local"> sends "2026-10-03T09:00" with no seconds and
// no zone, which encoding/json's time.Time refuses to parse.
func TestFlexibleTimeAcceptsDatetimeLocal(t *testing.T) {
	var payload struct {
		Start FlexibleTime `json:"start_at"`
		End   FlexibleTime `json:"end_at"`
	}
	if err := json.Unmarshal([]byte(`{"start_at":"2026-10-03T09:00","end_at":"2026-10-30T23:59"}`), &payload); err != nil {
		t.Fatalf("datetime-local rejected: %v", err)
	}
	if !payload.Start.Valid || payload.Start.Time.Hour() != 9 || payload.Start.Time.Minute() != 0 {
		t.Fatalf("start parsed wrong: %+v", payload.Start)
	}
	if !payload.End.Valid || payload.End.Time.Day() != 30 {
		t.Fatalf("end parsed wrong: %+v", payload.End)
	}
}

// TestFlexibleTimeAcceptsRFC3339 keeps the standard form working (API clients and
// the values the server itself returns).
func TestFlexibleTimeAcceptsRFC3339(t *testing.T) {
	var payload struct {
		Start FlexibleTime `json:"start_at"`
	}
	if err := json.Unmarshal([]byte(`{"start_at":"2026-10-03T09:00:00+08:00"}`), &payload); err != nil {
		t.Fatalf("rfc3339 rejected: %v", err)
	}
	if !payload.Start.Valid {
		t.Fatal("rfc3339 not marked valid")
	}
}

// TestFlexibleTimeEmptyIsNull keeps "no time set" meaning "unlimited" rather than
// an error or a zero time.
func TestFlexibleTimeEmptyIsNull(t *testing.T) {
	var payload struct {
		Start FlexibleTime `json:"start_at"`
	}
	if err := json.Unmarshal([]byte(`{"start_at":""}`), &payload); err != nil {
		t.Fatalf("empty rejected: %v", err)
	}
	if payload.Start.Valid || payload.Start.Ptr() != nil {
		t.Fatal("empty should mean unset")
	}
	if err := json.Unmarshal([]byte(`{"start_at":null}`), &payload); err != nil {
		t.Fatalf("null rejected: %v", err)
	}
	if payload.Start.Valid {
		t.Fatal("null should mean unset")
	}
	_ = time.Now
}
