package main

import (
	"sort"
	"time"
)

// State is the health state of a target.
type State string

const (
	StateHealthy State = "healthy"
	StateAlert   State = "alert"
)

// Transition records a state switch and the event that triggered it.
type Transition struct {
	Target     string    `json:"target"`
	Time       time.Time `json:"time"`
	From       State     `json:"from"`
	To         State     `json:"to"`
	EventID    string    `json:"event_id"`
	Line       int       `json:"line"`
	Suppressed bool      `json:"suppressed"`
}

// Notification is an alert/recovery message actually emitted (not suppressed).
type Notification struct {
	Target  string    `json:"target"`
	Time    time.Time `json:"time"`
	State   State     `json:"state"`
	EventID string    `json:"event_id"`
	Line    int       `json:"line"`
}

// FinalState is the ending health of a target after replay.
type FinalState struct {
	Target  string `json:"target"`
	State   State  `json:"state"`
	Healthy bool   `json:"healthy"`
}

// Report is the full replay output.
type Report struct {
	Transitions   []Transition   `json:"transitions"`
	Notifications []Notification `json:"notifications"`
	FinalStates   []FinalState   `json:"final_states"`
}

type targetState struct {
	target Target
	state  State
	streak int // consecutive same-result count toward the pending switch
}

func inMaintenance(t Target, ts time.Time) bool {
	for _, w := range t.Maintenance {
		if w.Contains(ts) {
			return true
		}
	}
	return false
}

// Replay processes events in (time, input-order) and builds the alert timeline.
func Replay(targets []Target, events []Event) Report {
	states := make(map[string]*targetState, len(targets))
	order := make([]string, 0, len(targets))
	for _, t := range targets {
		tc := t
		states[t.Name] = &targetState{target: tc, state: StateHealthy}
		order = append(order, t.Name)
	}

	sorted := make([]Event, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Time.Before(sorted[j].Time)
	})

	var transitions []Transition
	var notifications []Notification
	for _, ev := range sorted {
		ts := states[ev.Target]
		var to State
		switch {
		case ts.state == StateHealthy && ev.Result == ResultFailure:
			ts.streak++
			if ts.streak >= ts.target.FailureThreshold {
				to = StateAlert
			}
		case ts.state == StateAlert && ev.Result == ResultSuccess:
			ts.streak++
			if ts.streak >= ts.target.SuccessThreshold {
				to = StateHealthy
			}
		default:
			// Opposite result (or same result while already settled):
			// break the current streak.
			ts.streak = 0
		}
		if to == "" {
			continue
		}
		tr := Transition{
			Target:     ev.Target,
			Time:       ev.Time,
			From:       ts.state,
			To:         to,
			EventID:    ev.ID,
			Line:       ev.Line,
			Suppressed: inMaintenance(ts.target, ev.Time),
		}
		ts.state = to
		ts.streak = 0 // restart counting after a switch
		transitions = append(transitions, tr)
		if !tr.Suppressed {
			notifications = append(notifications, Notification{
				Target:  tr.Target,
				Time:    tr.Time,
				State:   tr.To,
				EventID: tr.EventID,
				Line:    tr.Line,
			})
		}
	}

	finals := make([]FinalState, 0, len(order))
	for _, name := range order {
		st := states[name].state
		finals = append(finals, FinalState{Target: name, State: st, Healthy: st == StateHealthy})
	}
	return Report{Transitions: transitions, Notifications: notifications, FinalStates: finals}
}
