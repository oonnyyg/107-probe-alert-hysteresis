package main

import (
	"sort"
	"time"
)

// State 是对象的健康状态。
type State string

const (
	StateHealthy  State = "healthy"
	StateAlerting State = "alerting"
)

// Transition 记录一次状态切换。
type Transition struct {
	Target     string
	From       State
	To         State
	Time       time.Time
	EventID    string
	Line       int
	Suppressed bool
}

// Notification 是一条实际发出的通知。
type Notification struct {
	Target  string
	Kind    State
	Time    time.Time
	EventID string
	Line    int
}

// FinalState 是对象在回放结束时的状态。
type FinalState struct {
	Target          string
	State           State
	Consecutive     int
	ConsecutiveKind string
}

// Report 是回放结果。
type Report struct {
	Transitions   []Transition
	Notifications []Notification
	FinalStates   []FinalState
}

type targetState struct {
	state       State
	consecutive int
	last        Result
	hasLast     bool
}

// Run 按 (时间, 输入顺序) 稳定处理事件并生成报告。
func Run(cfg *Config, events []Event) *Report {
	sorted := make([]Event, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].Time.Equal(sorted[j].Time) {
			return sorted[i].Time.Before(sorted[j].Time)
		}
		return sorted[i].Seq < sorted[j].Seq
	})

	states := map[string]*targetState{}
	for name := range cfg.Targets {
		states[name] = &targetState{state: StateHealthy}
	}

	rep := &Report{}
	for _, ev := range sorted {
		tgt := cfg.Targets[ev.Target]
		st := states[ev.Target]

		if st.hasLast && st.last == ev.Result {
			st.consecutive++
		} else {
			st.consecutive = 1
			st.last = ev.Result
			st.hasLast = true
		}

		var to State
		switch {
		case st.state == StateHealthy && ev.Result == ResultFailure &&
			st.consecutive >= tgt.Thresholds.Failure:
			to = StateAlerting
		case st.state == StateAlerting && ev.Result == ResultSuccess &&
			st.consecutive >= tgt.Thresholds.Recovery:
			to = StateHealthy
		default:
			continue
		}

		tr := Transition{
			Target:  ev.Target,
			From:    st.state,
			To:      to,
			Time:    ev.Time,
			EventID: ev.ID,
			Line:    ev.Line,
		}
		st.state = to
		st.consecutive = 0
		st.hasLast = false

		if tgt.InMaintenance(ev.Time) {
			tr.Suppressed = true
		} else {
			rep.Notifications = append(rep.Notifications, Notification{
				Target:  ev.Target,
				Kind:    to,
				Time:    ev.Time,
				EventID: ev.ID,
				Line:    ev.Line,
			})
		}
		rep.Transitions = append(rep.Transitions, tr)
	}

	for _, name := range cfg.Order {
		st := states[name]
		fs := FinalState{Target: name, State: st.state}
		if st.hasLast {
			fs.Consecutive = st.consecutive
			fs.ConsecutiveKind = string(st.last)
		}
		rep.FinalStates = append(rep.FinalStates, fs)
	}
	return rep
}
