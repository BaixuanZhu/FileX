package journal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// classDisabled is what guard reports when the guard has been switched off.
// Spelled here instead of imported: journal sits below guard and must not depend
// on it.
const classDisabled = "disabled"

// Group is one bucket of a count-only aggregation.
type Group struct {
	Key   string  `json:"key"`
	Count int     `json:"count"`
	Share float64 `json:"share"`
}

// SessionGroup is one session's totals. No path is carried: grouping by path
// would put file names on stdout, and grouping by session is what the decision
// gate needs.
type SessionGroup struct {
	SessionID  string `json:"session_id"`
	Source     string `json:"session_source"`
	Operations int    `json:"operations"`
	HighRisk   int    `json:"high_risk"`
	Denied     int    `json:"denied"`
	Bypassed   int    `json:"bypassed"`
	First      string `json:"first,omitempty"`
	Last       string `json:"last,omitempty"`
}

// ClassGroup is the decision chain for one guard class.
type ClassGroup struct {
	Class     string         `json:"class"`
	Records   int            `json:"records"`
	Decisions map[string]int `json:"decisions"`
	Bypassed  int            `json:"bypassed"`
	Denied    int            `json:"denied"`
}

// Gate holds the three numbers the observation period is defined on. They are
// computed over every matching record, not over the truncated session list.
type Gate struct {
	// SessionKeyShare is the fraction of matched records that carry a usable
	// session id. Below half, session-level limits cannot work in practice.
	SessionKeyShare  float64 `json:"session_key_share"`
	LabelledRecords  int     `json:"labelled_records"`
	DistinctSessions int     `json:"distinct_sessions"`
	// HighRiskSessions counts sessions that did at least one guarded operation.
	// The percentiles below are over those, so read them together with this.
	HighRiskSessions int `json:"high_risk_sessions"`
	HighRiskP50      int `json:"high_risk_p50"`
	HighRiskP99      int `json:"high_risk_p99"`
	HighRiskMax      int `json:"high_risk_max"`
	Denials          int `json:"denials"`
	Bypasses         int `json:"bypasses"`
	// Hosts are the distinct MCP client names seen. A distribution from one host
	// says nothing about whether the session key is generally available.
	Hosts []string `json:"hosts"`
}

// BypassSummary counts the records where a flag, not a person, approved.
type BypassSummary struct {
	Count    int      `json:"count"`
	Sessions []string `json:"sessions"`
}

// StatsOptions narrows what a scan looks at. A zero value reads the whole log.
type StatsOptions struct {
	Since   time.Time
	Until   time.Time
	Session string
	// Top bounds the session list. The other groupings come from fixed
	// vocabularies, so they are never truncated.
	Top int
}

// Report is the result of one scan.
type Report struct {
	Lines    int    `json:"lines"`
	Parsed   int    `json:"parsed_lines"`
	Skipped  int    `json:"skipped_lines"`
	Filtered int    `json:"filtered_records"`
	Matched  int    `json:"matched_records"`
	First    string `json:"first_timestamp,omitempty"`
	Last     string `json:"last_timestamp,omitempty"`

	Sources       []Group        `json:"sources"`
	Verbs         []Group        `json:"verbs"`
	Classes       []ClassGroup   `json:"classes"`
	Sessions      []SessionGroup `json:"sessions"`
	SessionsTotal int            `json:"sessions_total"`
	SessionsOmit  int            `json:"sessions_omitted"`
	Bypass        BypassSummary  `json:"bypass"`
	Gate          Gate           `json:"gate"`
}

// Collect reads the audit log and aggregates it. A missing log is an empty
// report: a fresh install has no history, which is an answer rather than a
// failure.
func Collect(opts StatsOptions) (*Report, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Scan(bytes.NewReader(nil), opts)
		}
		return nil, err
	}
	defer f.Close()
	return Scan(f, opts)
}

// Scan aggregates a JSONL stream. It reads once, keeps only counters, and takes
// no lock: a log being written while it is read yields a slightly older
// snapshot, which is all a distribution needs.
//
// A line that does not decode is counted in Skipped and the scan continues; a
// half-written last line is the normal case, not an error.
func Scan(r io.Reader, opts StatsOptions) (*Report, error) {
	a := newAccumulator(opts)
	scanner := bufio.NewScanner(r)
	// One record per line, but a long path or reason can exceed the default token
	// size, so the buffer is raised rather than letting a big line abort the scan.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		a.lines++
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			a.skipped++
			continue
		}
		a.parsed++
		a.add(e)
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			// A record past the buffer limit is unreadable; say so instead of
			// reporting a scan that saw everything.
			a.skipped++
			return a.finish(), nil
		}
		return nil, err
	}
	return a.finish(), nil
}

// Path is the audit log location. Unlike the write side it does not create the
// directory: finding no history must not leave a folder behind.
func Path() (string, error) {
	dir, err := journalDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

type sessionAcc struct {
	id         string
	source     string
	operations int
	highRisk   int
	denied     int
	bypassed   int
	first      time.Time
	last       time.Time
}

type accumulator struct {
	opts     StatsOptions
	lines    int
	parsed   int
	skipped  int
	filtered int
	matched  int

	sources  map[string]int
	verbs    map[string]int
	classes  map[string]*ClassGroup
	sessions map[string]*sessionAcc

	labelled  int
	denials   int
	bypasses  int
	hostNames map[string]bool
	first     time.Time
	last      time.Time
}

// unlabelledKey buckets records that carry no session id, so totals add up
// without inventing an identity for them.
const unlabelledKey = "\x00unlabelled"

func newAccumulator(opts StatsOptions) *accumulator {
	if opts.Top <= 0 {
		opts.Top = 20
	}
	return &accumulator{
		opts:      opts,
		sources:   map[string]int{},
		verbs:     map[string]int{},
		classes:   map[string]*ClassGroup{},
		sessions:  map[string]*sessionAcc{},
		hostNames: map[string]bool{},
	}
}

func (a *accumulator) add(e Entry) {
	ts, _ := time.Parse(time.RFC3339Nano, e.Timestamp)
	if !a.inWindow(ts) || (a.opts.Session != "" && e.SessionID != a.opts.Session) {
		a.filtered++
		return
	}
	a.matched++

	source := e.SessionSource
	if source == "" {
		source = SessionSourceAbsent
	}
	a.sources[source]++
	a.verbs[e.Op]++

	if isHighRisk(e) {
		a.class(e)
	}
	if e.Result == "denied" {
		a.denials++
	}
	if e.Bypass {
		a.bypasses++
	}
	if e.SessionID != "" {
		a.labelled++
	}
	if host, ok := mcpHost(e.Actor); ok {
		a.hostNames[host] = true
	}

	key := e.SessionID
	if key == "" {
		key = unlabelledKey
	}
	s := a.sessions[key]
	if s == nil {
		s = &sessionAcc{id: e.SessionID, source: source}
		a.sessions[key] = s
	}
	s.operations++
	if isHighRisk(e) {
		s.highRisk++
	}
	if e.Result == "denied" {
		s.denied++
	}
	if e.Bypass {
		s.bypassed++
	}
	s.track(ts)
	a.trackSpan(ts)
}

func (a *accumulator) class(e Entry) {
	group := a.classes[e.GuardClass]
	if group == nil {
		group = &ClassGroup{Class: e.GuardClass, Decisions: map[string]int{}}
		a.classes[e.GuardClass] = group
	}
	group.Records++
	decision := e.Decision
	if decision == "" {
		decision = "unrecorded"
	}
	group.Decisions[decision]++
	if e.Bypass {
		group.Bypassed++
	}
	if e.Result == "denied" {
		group.Denied++
	}
}

func (a *accumulator) inWindow(ts time.Time) bool {
	if ts.IsZero() {
		// A record without a readable timestamp cannot be placed in a window, so
		// no window excludes it.
		return true
	}
	if !a.opts.Since.IsZero() && ts.Before(a.opts.Since) {
		return false
	}
	if !a.opts.Until.IsZero() && ts.After(a.opts.Until) {
		return false
	}
	return true
}

func (a *accumulator) trackSpan(ts time.Time) {
	if ts.IsZero() {
		return
	}
	if a.first.IsZero() || ts.Before(a.first) {
		a.first = ts
	}
	if ts.After(a.last) {
		a.last = ts
	}
}

func (s *sessionAcc) track(ts time.Time) {
	if ts.IsZero() {
		return
	}
	if s.first.IsZero() || ts.Before(s.first) {
		s.first = ts
	}
	if ts.After(s.last) {
		s.last = ts
	}
}

func (a *accumulator) finish() *Report {
	out := &Report{
		Lines:    a.lines,
		Parsed:   a.parsed,
		Skipped:  a.skipped,
		Filtered: a.filtered,
		Matched:  a.matched,
		Sources:  groups(a.sources, a.matched),
		Verbs:    groups(a.verbs, a.matched),
		Classes:  classList(a.classes),
		Bypass:   BypassSummary{Count: a.bypasses},
	}
	if !a.first.IsZero() {
		out.First = a.first.Format(time.RFC3339Nano)
	}
	if !a.last.IsZero() {
		out.Last = a.last.Format(time.RFC3339Nano)
	}

	all := sessionList(a.sessions)
	out.SessionsTotal = len(all)

	risk := make([]int, 0, len(all))
	for _, s := range all {
		if s.HighRisk > 0 {
			risk = append(risk, s.HighRisk)
		}
	}
	out.Gate = Gate{
		SessionKeyShare:  share(a.labelled, a.matched),
		LabelledRecords:  a.labelled,
		DistinctSessions: len(all),
		HighRiskSessions: len(risk),
		Denials:          a.denials,
		Bypasses:         a.bypasses,
		Hosts:            sortedKeys(a.hostNames),
	}
	out.Gate.HighRiskP50, out.Gate.HighRiskP99, out.Gate.HighRiskMax = percentiles(risk)

	out.Bypass.Sessions = bypassedSessions(all)
	if len(all) > a.opts.Top {
		out.Sessions = all[:a.opts.Top]
		out.SessionsOmit = len(all) - a.opts.Top
	} else {
		out.Sessions = all
	}
	return out
}

func bypassedSessions(all []SessionGroup) []string {
	out := make([]string, 0, len(all))
	for _, s := range all {
		if s.Bypassed > 0 && s.SessionID != "" {
			out = append(out, s.SessionID)
		}
	}
	sort.Strings(out)
	return out
}

// isHighRisk is the guard's own objection, not the outcome: a bypassed purge is
// still a permanent delete and has to show up in the distribution.
func isHighRisk(e Entry) bool {
	return e.GuardClass != "" && e.GuardClass != classDisabled
}

func mcpHost(actor string) (string, bool) {
	if !strings.HasPrefix(actor, "mcp:") {
		return "", false
	}
	name := strings.TrimPrefix(actor, "mcp:")
	if name == "" {
		return "", false
	}
	return name, true
}

func sessionList(byKey map[string]*sessionAcc) []SessionGroup {
	out := make([]SessionGroup, 0, len(byKey))
	for _, s := range byKey {
		out = append(out, SessionGroup{
			SessionID:  s.id,
			Source:     s.source,
			Operations: s.operations,
			HighRisk:   s.highRisk,
			Denied:     s.denied,
			Bypassed:   s.bypassed,
			First:      formatTime(s.first),
			Last:       formatTime(s.last),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Operations != out[j].Operations {
			return out[i].Operations > out[j].Operations
		}
		if out[i].SessionID != out[j].SessionID {
			return out[i].SessionID < out[j].SessionID
		}
		return out[i].Source < out[j].Source
	})
	return out
}

func classList(byClass map[string]*ClassGroup) []ClassGroup {
	out := make([]ClassGroup, 0, len(byClass))
	for _, g := range byClass {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Records != out[j].Records {
			return out[i].Records > out[j].Records
		}
		return out[i].Class < out[j].Class
	})
	return out
}

func formatTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format(time.RFC3339Nano)
}

// share returns a fraction of a total. A zero total has no percentage.
func share(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(int(float64(part)/float64(total)*1000+0.5)) / 1000
}

func groups(counts map[string]int, total int) []Group {
	out := make([]Group, 0, len(counts))
	for key, count := range counts {
		out = append(out, Group{Key: key, Count: count, Share: share(count, total)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func percentiles(values []int) (p50, p99, max int) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	sorted := make([]int, len(values))
	copy(sorted, values)
	sort.Ints(sorted)
	max = sorted[len(sorted)-1]
	p50 = sorted[percentileIndex(len(sorted), 0.50)]
	p99 = sorted[percentileIndex(len(sorted), 0.99)]
	return p50, p99, max
}

// percentileIndex rounds up, so a single-sample p99 is that sample rather than
// the value below it.
func percentileIndex(n int, p float64) int {
	idx := int(math.Ceil(p*float64(n))) - 1
	if idx < 0 {
		return 0
	}
	if idx >= n {
		return n - 1
	}
	return idx
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
