package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Result is the outcome of a single probe.
type Result string

const (
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
)

// Event is one probe observation from the replay log.
type Event struct {
	ID     string
	Target string
	Time   time.Time
	Result Result
	Line   int // 1-based line number in the input file
	Order  int // global input order across kept events
}

type eventJSON struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	Time   string `json:"time"`
	Result string `json:"result"`
}

// LoadEvents parses JSONL probe events from r, validating against known
// targets. Identical duplicate IDs are collapsed; conflicting duplicates
// abort the whole batch. Any error references the 1-based input line.
func LoadEvents(r io.Reader, known map[string]bool) ([]Event, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	byID := map[string]Event{}
	var events []Event
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var ej eventJSON
		if err := json.Unmarshal([]byte(text), &ej); err != nil {
			return nil, fmt.Errorf("line %d: invalid JSON: %v", line, err)
		}
		if ej.ID == "" {
			return nil, fmt.Errorf("line %d: event id must not be empty", line)
		}
		if !known[ej.Target] {
			return nil, fmt.Errorf("line %d: unknown target %q", line, ej.Target)
		}
		ts, err := time.Parse(time.RFC3339, ej.Time)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid time %q: %v", line, ej.Time, err)
		}
		res := Result(ej.Result)
		if res != ResultSuccess && res != ResultFailure {
			return nil, fmt.Errorf("line %d: invalid result %q (want %q or %q)", line, ej.Result, ResultSuccess, ResultFailure)
		}
		ev := Event{ID: ej.ID, Target: ej.Target, Time: ts, Result: res, Line: line}
		if prev, dup := byID[ev.ID]; dup {
			if prev.Target == ev.Target && prev.Time.Equal(ev.Time) && prev.Result == ev.Result {
				continue // identical duplicate: process once
			}
			return nil, fmt.Errorf("line %d: event id %q conflicts with line %d (same id, different content)", line, ev.ID, prev.Line)
		}
		ev.Order = len(events)
		byID[ev.ID] = ev
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading events: %w", err)
	}
	return events, nil
}
