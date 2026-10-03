package main

import (
	"strings"
	"testing"
	"time"
)

func mustConfig(t *testing.T, jsonCfg string) []Target {
	t.Helper()
	targets, err := LoadConfig(strings.NewReader(jsonCfg))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return targets
}

func mustEvents(t *testing.T, targets []Target, jsonl string) []Event {
	t.Helper()
	known := map[string]bool{}
	for _, tg := range targets {
		known[tg.Name] = true
	}
	events, err := LoadEvents(strings.NewReader(jsonl), known)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	return events
}

func ts(sec int) string {
	return time.Date(2026, 1, 1, 0, 0, sec, 0, time.UTC).Format(time.RFC3339)
}

const basicConfig = `{"targets":[{"name":"api","failure_threshold":3,"success_threshold":2}]}`

func TestThresholdBoundary(t *testing.T) {
	targets := mustConfig(t, basicConfig)
	events := mustEvents(t, targets,
		`{"id":"e1","target":"api","time":"`+ts(1)+`","result":"failure"}`+"\n"+
			`{"id":"e2","target":"api","time":"`+ts(2)+`","result":"failure"}`+"\n")
	rep := Replay(targets, events)
	if len(rep.Transitions) != 0 {
		t.Fatalf("expected no transition below threshold, got %+v", rep.Transitions)
	}
	if !rep.FinalStates[0].Healthy {
		t.Fatalf("expected healthy, got %+v", rep.FinalStates[0])
	}

	events = mustEvents(t, targets,
		`{"id":"e1","target":"api","time":"`+ts(1)+`","result":"failure"}`+"\n"+
			`{"id":"e2","target":"api","time":"`+ts(2)+`","result":"failure"}`+"\n"+
			`{"id":"e3","target":"api","time":"`+ts(3)+`","result":"failure"}`+"\n")
	rep = Replay(targets, events)
	if len(rep.Transitions) != 1 || rep.Transitions[0].To != StateAlert {
		t.Fatalf("expected one alert transition, got %+v", rep.Transitions)
	}
	if rep.Transitions[0].EventID != "e3" {
		t.Fatalf("transition should trace to e3, got %q", rep.Transitions[0].EventID)
	}
}

func TestRecoveryHysteresisAndStreakReset(t *testing.T) {
	targets := mustConfig(t, basicConfig)
	lines := []string{
		`{"id":"f1","target":"api","time":"` + ts(1) + `","result":"failure"}`,
		`{"id":"f2","target":"api","time":"` + ts(2) + `","result":"failure"}`,
		`{"id":"f3","target":"api","time":"` + ts(3) + `","result":"failure"}`,
		`{"id":"s1","target":"api","time":"` + ts(4) + `","result":"success"}`,
		`{"id":"f4","target":"api","time":"` + ts(5) + `","result":"failure"}`,
		`{"id":"s2","target":"api","time":"` + ts(6) + `","result":"success"}`,
		`{"id":"s3","target":"api","time":"` + ts(7) + `","result":"success"}`,
		`{"id":"f5","target":"api","time":"` + ts(8) + `","result":"failure"}`,
		`{"id":"f6","target":"api","time":"` + ts(9) + `","result":"failure"}`,
		`{"id":"f7","target":"api","time":"` + ts(10) + `","result":"failure"}`,
	}
	rep := Replay(targets, mustEvents(t, targets, strings.Join(lines, "\n")+"\n"))
	if len(rep.Transitions) != 3 {
		t.Fatalf("expected 3 transitions, got %+v", rep.Transitions)
	}
	if rep.Transitions[1].To != StateHealthy || rep.Transitions[1].EventID != "s3" {
		t.Fatalf("expected recovery at s3, got %+v", rep.Transitions[1])
	}
	if rep.Transitions[2].To != StateAlert || rep.Transitions[2].EventID != "f7" {
		t.Fatalf("expected re-alert at f7, got %+v", rep.Transitions[2])
	}
	if len(rep.Notifications) != 3 {
		t.Fatalf("expected 3 notifications, got %+v", rep.Notifications)
	}
}
