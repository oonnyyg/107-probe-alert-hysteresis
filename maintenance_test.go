package main

import (
	"strings"
	"testing"
)

const maintConfig = `{"targets":[{
  "name":"db",
  "failure_threshold":2,
  "success_threshold":1,
  "maintenance":[{"start":"2026-01-01T00:00:05Z","end":"2026-01-01T00:00:09Z"}]
}]}`

func TestMaintenanceBoundariesAndSuppression(t *testing.T) {
	targets := mustConfig(t, maintConfig)
	lines := []string{
		// t=4: before window, not suppressed
		`{"id":"a","target":"db","time":"` + ts(4) + `","result":"failure"}`,
		// t=5: window start is inclusive -> suppressed
		`{"id":"b","target":"db","time":"` + ts(5) + `","result":"failure"}`,
		// t=8: inside window, still updating state, suppressed
		`{"id":"c","target":"db","time":"` + ts(8) + `","result":"failure"}`,
		// t=9: window end is exclusive -> not suppressed
		`{"id":"d","target":"db","time":"` + ts(9) + `","result":"failure"}`,
	}
	rep := Replay(targets, mustEvents(t, targets, strings.Join(lines, "\n")+"\n"))
	if len(rep.Transitions) != 1 {
		t.Fatalf("expected 1 transition, got %+v", rep.Transitions)
	}
	tr := rep.Transitions[0]
	if tr.EventID != "b" || !tr.Suppressed {
		t.Fatalf("expected suppressed transition triggered by b, got %+v", tr)
	}
	if len(rep.Notifications) != 0 {
		t.Fatalf("expected no notifications (suppressed, no backfill), got %+v", rep.Notifications)
	}
	if rep.FinalStates[0].State != StateAlert {
		t.Fatalf("state must keep updating during maintenance, got %+v", rep.FinalStates[0])
	}
}

func TestMaintenanceEndBoundaryRecoveryNotSuppressed(t *testing.T) {
	targets := mustConfig(t, maintConfig)
	lines := []string{
		`{"id":"a","target":"db","time":"` + ts(5) + `","result":"failure"}`,
		`{"id":"b","target":"db","time":"` + ts(6) + `","result":"failure"}`, // alert, suppressed
		`{"id":"c","target":"db","time":"` + ts(9) + `","result":"success"}`, // end exclusive: recover, notified
	}
	rep := Replay(targets, mustEvents(t, targets, strings.Join(lines, "\n")+"\n"))
	if len(rep.Transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %+v", rep.Transitions)
	}
	if rep.Transitions[1].Suppressed {
		t.Fatalf("transition at window end must not be suppressed: %+v", rep.Transitions[1])
	}
	if len(rep.Notifications) != 1 || rep.Notifications[0].State != StateHealthy {
		t.Fatalf("expected one recovery notification, got %+v", rep.Notifications)
	}
}

func TestDuplicateEvents(t *testing.T) {
	targets := mustConfig(t, basicConfig)
	// Identical duplicate: processed once, so only 2 effective failures -> no alert.
	dup := `{"id":"e1","target":"api","time":"` + ts(1) + `","result":"failure"}` + "\n" +
		`{"id":"e1","target":"api","time":"` + ts(1) + `","result":"failure"}` + "\n" +
		`{"id":"e2","target":"api","time":"` + ts(2) + `","result":"failure"}` + "\n"
	events, err := LoadEvents(strings.NewReader(dup), map[string]bool{"api": true})
	if err != nil {
		t.Fatalf("identical duplicates must be accepted: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected duplicate collapsed to 2 events, got %d", len(events))
	}
	if rep := Replay(targets, events); len(rep.Transitions) != 0 {
		t.Fatalf("duplicate must not double-count, got %+v", rep.Transitions)
	}

	// Conflicting duplicate: whole batch rejected with line numbers.
	conflict := `{"id":"e1","target":"api","time":"` + ts(1) + `","result":"failure"}` + "\n" +
		`{"id":"e1","target":"api","time":"` + ts(2) + `","result":"success"}` + "\n"
	if _, err := LoadEvents(strings.NewReader(conflict), map[string]bool{"api": true}); err == nil ||
		!strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("expected conflict error citing both lines, got %v", err)
	}
}

func TestInputValidationLineNumbers(t *testing.T) {
	if _, err := LoadConfig(strings.NewReader(`{"targets":[{"name":"x","failure_threshold":0,"success_threshold":1}]}`)); err == nil ||
		!strings.Contains(err.Error(), "positive integer") {
		t.Fatalf("expected positive-integer threshold error, got %v", err)
	}
	if _, err := LoadConfig(strings.NewReader(`{"targets":[{"name":"x","failure_threshold":1,"success_threshold":-2}]}`)); err == nil {
		t.Fatalf("expected negative success_threshold to be rejected")
	}
	known := map[string]bool{"api": true}
	bad := `{"id":"e1","target":"api","time":"` + ts(1) + `","result":"failure"}` + "\n" +
		`{"id":"e2","target":"ghost","time":"` + ts(2) + `","result":"failure"}` + "\n" +
		`{"id":"e3","target":"api","time":"` + ts(3) + `","result":"flaky"}` + "\n"
	if _, err := LoadEvents(strings.NewReader(bad), known); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("expected unknown target at line 2, got %v", err)
	}
	bad = `{"id":"e1","target":"api","time":"` + ts(1) + `","result":"flaky"}` + "\n"
	if _, err := LoadEvents(strings.NewReader(bad), known); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("expected invalid result at line 1, got %v", err)
	}
}

func TestStableOrderingSameTimestamp(t *testing.T) {
	targets := mustConfig(t, `{"targets":[
	  {"name":"a","failure_threshold":1,"success_threshold":1},
	  {"name":"b","failure_threshold":1,"success_threshold":1}
	]}`)
	// Same timestamp for both targets; file order must be preserved.
	lines := []string{
		`{"id":"b1","target":"b","time":"` + ts(1) + `","result":"failure"}`,
		`{"id":"a1","target":"a","time":"` + ts(1) + `","result":"failure"}`,
	}
	rep := Replay(targets, mustEvents(t, targets, strings.Join(lines, "\n")+"\n"))
	if len(rep.Transitions) != 2 || rep.Transitions[0].Target != "b" || rep.Transitions[1].Target != "a" {
		t.Fatalf("expected stable input order for equal timestamps, got %+v", rep.Transitions)
	}
}
