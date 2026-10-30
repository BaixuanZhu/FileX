package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"filex/internal/errs"
	"filex/internal/journal"
)

// runJournal is read-only: it never opens the log for writing, never takes a
// lock, and a failure here cannot stop a file operation because nothing on the
// write path calls it.
func runJournal(args []string) *errs.Error {
	if len(args) < 1 {
		return errs.New(errs.EUsage, "usage: filex journal stats [--since=<dur|RFC3339>] [--until=<...>] [--session=<id>] [--group-by=session|verb|class|source] [--top=<n>] [--json]")
	}
	switch args[0] {
	case "stats":
		return runJournalStats(args[1:])
	case "path":
		path, err := journal.Path()
		if err != nil {
			return errs.Newf(errs.EIO, "cannot locate the audit log: %v", err)
		}
		if err := errs.WriteSuccess(os.Stdout, map[string]string{"path": path}); err != nil {
			return errs.Newf(errs.EIO, "write failed: %v", err)
		}
		return nil
	default:
		return errs.Newf(errs.EUsage, "unknown journal subcommand: %s (only stats and path exist)", args[0])
	}
}

type statsFlags struct {
	journal.StatsOptions
	groupBy string
	asJSON  bool
}

func runJournalStats(args []string) *errs.Error {
	bounds := newBounds()
	opts := statsFlags{StatsOptions: journal.StatsOptions{Top: 20}}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.asJSON = true
		case strings.HasPrefix(arg, "--since="):
			t, err := parseBound(arg[len("--since="):], bounds)
			if err != nil {
				return err
			}
			opts.Since = t
		case strings.HasPrefix(arg, "--until="):
			t, err := parseBound(arg[len("--until="):], bounds)
			if err != nil {
				return err
			}
			opts.Until = t
		case arg == "--since" || arg == "--until":
			return errs.Newf(errs.EUsage, "%s needs a value: --%s=72h or --%s=2026-10-01T00:00:00Z", arg, arg, arg)
		case strings.HasPrefix(arg, "--session="):
			opts.Session = arg[len("--session="):]
		case strings.HasPrefix(arg, "--group-by="):
			opts.groupBy = arg[len("--group-by="):]
		case strings.HasPrefix(arg, "--top="):
			n := atoi(arg[len("--top="):])
			if n <= 0 {
				return errs.Newf(errs.EUsage, "--top must be a positive integer, got %q", arg[len("--top="):])
			}
			opts.Top = n
		default:
			return errs.Newf(errs.EUsage, "unknown flag for journal stats: %s", arg)
		}
	}

	switch opts.groupBy {
	case "", "session", "verb", "class", "source":
	default:
		return errs.Newf(errs.EUsage, "--group-by must be session, verb, class or source, got %q", opts.groupBy)
	}

	report, err := journal.Collect(opts.StatsOptions)
	if err != nil {
		return errs.Newf(errs.EIO, "cannot read the audit log: %v", err)
	}

	if opts.asJSON {
		if err := errs.WriteSuccess(os.Stdout, report); err != nil {
			return errs.Newf(errs.EIO, "write failed: %v", err)
		}
		return nil
	}
	return writeStats(os.Stdout, report, opts.groupBy)
}

// newBounds anchors relative values at one instant, so --since and --until
// measured in the same call agree even if the scan runs across a second.
func newBounds() time.Time { return time.Now() }

// parseBound accepts a duration counted back from now, or an absolute timestamp.
// A duration is the common case ("what happened today"); a timestamp is what the
// decision gate needs once a period has to be reproducible. Both flags read the
// same way, so --since=24h --until=12h means the day between those two points.
func parseBound(value string, now time.Time) (time.Time, *errs.Error) {
	if value == "" {
		return time.Time{}, errs.New(errs.EUsage, "empty time value")
	}
	if d, err := time.ParseDuration(value); err == nil {
		return now.Add(-d), nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errs.WithHint(
		errs.Newf(errs.EUsage, "unrecognised time value %q", value),
		"use a duration like 72h, a date like 2026-10-01, or an RFC3339 timestamp",
	)
}

func writeStats(w io.Writer, r *journal.Report, groupBy string) *errs.Error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "audit log\t%s\n", logLocation())
	fmt.Fprintf(tw, "records\t%d lines, %d parsed, %d skipped, %d matched, %d filtered\n",
		r.Lines, r.Parsed, r.Skipped, r.Matched, r.Filtered)
	if r.First != "" || r.Last != "" {
		fmt.Fprintf(tw, "span\t%s .. %s\n", dash(r.First), dash(r.Last))
	}
	if err := tw.Flush(); err != nil {
		return errs.Newf(errs.EIO, "write failed: %v", err)
	}

	sections := []section{writeGate, writeSources, writeSessions, writeVerbs, writeClasses, writeBypass}
	if groupBy != "" {
		byName := map[string]section{"session": writeSessions, "verb": writeVerbs, "class": writeClasses, "source": writeSources}
		sections = []section{byName[groupBy]}
	}

	for _, s := range sections {
		if err := s(tw, r); err != nil {
			return errs.Newf(errs.EIO, "write failed: %v", err)
		}
		if err := tw.Flush(); err != nil {
			return errs.Newf(errs.EIO, "write failed: %v", err)
		}
	}
	return nil
}

type section func(*tabwriter.Writer, *journal.Report) error

func logLocation() string {
	path, err := journal.Path()
	if err != nil {
		return "(unknown home directory)"
	}
	return path
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// writeGate prints the three inputs the decision gate is defined on. The
// thresholds themselves live in the task docs, not here: this reports what was
// measured and does not judge it.
func writeGate(tw *tabwriter.Writer, r *journal.Report) error {
	g := r.Gate
	fmt.Fprintf(tw, "\ndecision gate\n")
	fmt.Fprintf(tw, "  session key share\t%.3f  (%d of %d records carry a session id)\n", g.SessionKeyShare, g.LabelledRecords, r.Matched)
	fmt.Fprintf(tw, "  distinct sessions\t%d  (%d labelled)\n", g.DistinctSessions, g.LabelledRecords)
	fmt.Fprintf(tw, "  high-risk per session\tp50=%d  p99=%d  max=%d  over %d sessions\n",
		g.HighRiskP50, g.HighRiskP99, g.HighRiskMax, g.HighRiskSessions)
	fmt.Fprintf(tw, "  denials\t%d\n", g.Denials)
	fmt.Fprintf(tw, "  bypasses\t%d\n", g.Bypasses)
	fmt.Fprintf(tw, "  mcp hosts seen\t%s\n", joinOr(g.Hosts, "(none)"))
	return nil
}

func writeSources(tw *tabwriter.Writer, r *journal.Report) error {
	fmt.Fprintf(tw, "\nsession source\n")
	fmt.Fprintf(tw, "  SOURCE\tRECORDS\tSHARE\n")
	for _, s := range r.Sources {
		fmt.Fprintf(tw, "  %s\t%d\t%.3f\n", s.Key, s.Count, s.Share)
	}
	return nil
}

func writeSessions(tw *tabwriter.Writer, r *journal.Report) error {
	label := "sessions"
	if r.SessionsOmit > 0 {
		label = fmt.Sprintf("sessions (top %d, %d more not shown)", len(r.Sessions), r.SessionsOmit)
	}
	fmt.Fprintf(tw, "\n%s of %d\n", label, r.SessionsTotal)
	fmt.Fprintf(tw, "  SESSION\tSOURCE\tOPS\tHIGH-RISK\tDENIED\tBYPASSED\tFIRST\tLAST\n")
	for _, s := range r.Sessions {
		fmt.Fprintf(tw, "  %s\t%s\t%d\t%d\t%d\t%d\t%s\t%s\n",
			shortSession(s.SessionID), s.Source, s.Operations, s.HighRisk, s.Denied, s.Bypassed,
			shortStamp(s.First), shortStamp(s.Last))
	}
	return nil
}

func writeVerbs(tw *tabwriter.Writer, r *journal.Report) error {
	fmt.Fprintf(tw, "\nverbs\n")
	fmt.Fprintf(tw, "  VERB\tRECORDS\tSHARE\n")
	for _, v := range r.Verbs {
		fmt.Fprintf(tw, "  %s\t%d\t%.3f\n", dash(v.Key), v.Count, v.Share)
	}
	return nil
}

func writeClasses(tw *tabwriter.Writer, r *journal.Report) error {
	fmt.Fprintf(tw, "\nguard classes\n")
	if len(r.Classes) == 0 {
		fmt.Fprintf(tw, "  (no guarded operation in this window)\n")
		return nil
	}
	fmt.Fprintf(tw, "  CLASS\tRECORDS\tDECISIONS\tDENIED\tBYPASSED\n")
	for _, c := range r.Classes {
		fmt.Fprintf(tw, "  %s\t%d\t%s\t%d\t%d\n", c.Class, c.Records, decisions(c.Decisions), c.Denied, c.Bypassed)
	}
	return nil
}

func writeBypass(tw *tabwriter.Writer, r *journal.Report) error {
	fmt.Fprintf(tw, "\nbypass\n")
	fmt.Fprintf(tw, "  records approved by flag\t%d\n", r.Bypass.Count)
	if len(r.Bypass.Sessions) > 0 {
		fmt.Fprintf(tw, "  sessions involved\t%s\n", joinShort(r.Bypass.Sessions))
	}
	return nil
}

func decisions(counts map[string]int) string {
	if len(counts) == 0 {
		return "-"
	}
	names := make([]string, 0, len(counts))
	for k := range counts {
		names = append(names, k)
	}
	// Sorted so two runs over the same log print the same line.
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", n, counts[n]))
	}
	return strings.Join(parts, " ")
}

// shortSession keeps a 32-character id readable in a table. Collisions within the
// displayed prefix are possible; --json carries the full id.
func shortSession(id string) string {
	if id == "" {
		return "(no session)"
	}
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "…"
}

func shortStamp(s string) string {
	if len(s) >= 19 {
		return s[:19]
	}
	return dash(s)
}

func joinOr(items []string, empty string) string {
	if len(items) == 0 {
		return empty
	}
	return strings.Join(items, ", ")
}

func joinShort(items []string) string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		out = append(out, shortSession(s))
	}
	return strings.Join(out, ", ")
}
