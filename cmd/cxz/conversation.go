//go:build !windows

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

// A month is the window the search walks back through, because that is the unit
// a person asks in: what did we say about this recently, and then -- if it was
// not there -- the month before that.
const searchWindow = 30 * 24 * time.Hour

func conversationCommand() *xli.Command {
	group := &xli.Command{Name: "conversation", Brief: "Search what was said, across every project and session", Handler: xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error {
		return c.PrintHelp(c.Writer)
	})}
	group.Commands = xli.Commands{{
		Name:  "search",
		Brief: "Search conversations newest first, one window of time at a time",
		Synop: "Searches the last 30 days by default and prints the cursor to continue with. --continue\n" +
			"CURSOR reads the window before the one just read, so a question that was not answered\n" +
			"recently can be followed backwards without re-reading what was already seen.\n\n" +
			"Answers come from the installation's conversation index, which is kept up to date as\n" +
			"events are recorded. A conversation the index has not reached is reported rather than\n" +
			"left out silently; --progress shows what it caught up on.",
		Args: arg.Args{stringArg("QUERY", true)},
		Flags: flg.Flags{
			&flg.Base[string, matchParser]{Name: "match", Brief: "How to match: substring, regex (RE2) or fuzzy (per line)", Default: ptr("substring")},
			switchFlag("ignore-case", "Match regardless of case"),
			switchFlag("tools", "Search tool calls and their results too"),
			&flg.Strings{Name: "project", Brief: "Search only this project (repeatable; name, alias or id)"},
			&flg.Strings{Name: "exclude", Brief: "Skip this project (repeatable; name, alias or id)"},
			stringFlag("since", "Oldest time to search: RFC3339, a date, or a duration such as 90d ago", ""),
			stringFlag("until", "Newest time to search, exclusive: RFC3339, a date, or a duration ago", ""),
			stringFlag("window", "How far back one search goes when --since is absent (default 30d)", ""),
			stringFlag("continue", "Cursor from an earlier search: reads the window before it", ""),
			&flg.Int{Name: "limit", Brief: "Most hits to return (default 200)", Default: ptr(0)},
			&flg.Int{Name: "snippet", Brief: "Matching text to show per hit, in bytes (0 for none)", Default: ptr(0)},
			switchFlag("progress", "Report what the index caught up on before answering"),
			formatFlag(),
		},
		Handler: withClient(conversationSearch),
	}}
	return group
}

type matchParser struct{ flg.StringParser }

func (matchParser) Parse(v string) (string, error) {
	if v != "substring" && v != "regex" && v != "fuzzy" {
		return "", fmt.Errorf("match must be substring, regex or fuzzy")
	}
	return v, nil
}

func conversationSearch(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
	r := &api.SearchRequest{
		Query:        arg.MustGet[string](c, "QUERY"),
		Match:        flg.MustGet[string](c, "match"),
		IgnoreCase:   flg.MustGet[bool](c, "ignore-case"),
		IncludeTools: flg.MustGet[bool](c, "tools"),
		Limit:        int32(flg.MustGet[int](c, "limit")),
		Snippet:      int32(flg.MustGet[int](c, "snippet")),
		Cursor:       flg.MustGet[string](c, "continue"),
		ClientId:     core.ID(),
	}
	projects, _ := flg.Get[[]string](c, "project")
	exclude, _ := flg.Get[[]string](c, "exclude")
	now := time.Now().UTC()
	if r.Cursor == "" {
		window := searchWindow
		if v := flg.MustGet[string](c, "window"); v != "" {
			d, err := parseSpan(v)
			if err != nil {
				return err
			}
			window = d
		}
		until, err := parseWhen(flg.MustGet[string](c, "until"), now)
		if err != nil {
			return fmt.Errorf("--until: %w", err)
		}
		if until.IsZero() {
			until = now
		}
		since, err := parseWhen(flg.MustGet[string](c, "since"), now)
		if err != nil {
			return fmt.Errorf("--since: %w", err)
		}
		if since.IsZero() {
			since = until.Add(-window)
		}
		if !since.Before(until) {
			return fmt.Errorf("--since must be before --until")
		}
		r.SinceMs, r.UntilMs = since.UnixMilli(), until.UnixMilli()
	}
	// Names are resolved here, where they were displayed, rather than guessed
	// at by the server.
	if len(projects) > 0 || len(exclude) > 0 {
		known, err := client.Projects(ctx, &api.Empty{})
		if err != nil {
			return err
		}
		if r.Projects, err = resolveProjects(known.Projects, projects); err != nil {
			return err
		}
		if r.Exclude, err = resolveProjects(known.Projects, exclude); err != nil {
			return err
		}
	}

	stream, err := client.Search(ctx, r)
	if err != nil {
		return err
	}
	json := flg.MustGet[string](c, "format") == "json"
	progress := flg.MustGet[bool](c, "progress")
	for {
		reply, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		switch {
		case reply.Visit != nil:
			if json {
				if err = writeJSON(c.Writer, reply.Visit); err != nil {
					return err
				}
				continue
			}
			if err = printVisit(c, reply.Visit); err != nil {
				return err
			}
		case reply.Progress != nil:
			if json {
				if err = writeJSON(c.Writer, reply.Progress); err != nil {
					return err
				}
				continue
			}
			if progress {
				fmt.Fprintf(c.ErrWriter, "%s %s %s\n", reply.Progress.State, projectLabel(reply.Progress), reply.Progress.Message)
			}
		case reply.Summary != nil:
			if json {
				return writeJSON(c.Writer, reply.Summary)
			}
			return printSummary(c, reply.Summary, r.Query)
		}
	}
}

func projectLabel(p *api.SearchProgress) string {
	if p.ProjectName != "" {
		return p.ProjectName
	}
	return p.ProjectId
}

// printVisit puts the conversation on one line and its matches under it, which
// is the shape of the answer: a few places where this was discussed, and when.
func printVisit(c *xli.Command, v *api.SearchVisit) error {
	if len(v.Hits) == 0 {
		return nil
	}
	name := v.Title
	if v.Alias != "" {
		name = v.Alias
		if v.Title != "" {
			name += " · " + v.Title
		}
	}
	if name == "" {
		name = v.SessionId
	}
	if _, err := fmt.Fprintf(c.Writer, "\n%s", name); err != nil {
		return err
	}
	// A host-local installation has no projects to name, and an empty column
	// would be the only trace of that.
	if where := v.ProjectName; where != "" || v.ProjectId != "" {
		if where == "" {
			where = v.ProjectId
		}
		fmt.Fprintf(c.Writer, "  %s", where)
	}
	if v.Agent != "" {
		fmt.Fprintf(c.Writer, "  %s", v.Agent)
	}
	if v.Truncated {
		fmt.Fprint(c.Writer, "  (older events trimmed)")
	}
	fmt.Fprintf(c.Writer, "  %s\n", v.SessionId)
	for _, h := range v.Hits {
		when := time.UnixMilli(h.TimeMs).Local().Format("2006-01-02 15:04")
		if _, err := fmt.Fprintf(c.Writer, "  %s  %-9s #%-6d %s\n", when, h.Kind, h.Seq, h.Snippet); err != nil {
			return err
		}
	}
	return nil
}

// printSummary reads the window off the reply rather than the request: a
// continued page sent no window, because the cursor was carrying it.
func printSummary(c *xli.Command, s *api.SearchSummary, query string) error {
	window := ""
	if s.SinceMs > 0 && s.UntilMs > 0 {
		window = fmt.Sprintf(" in %s..%s", time.UnixMilli(s.SinceMs).Local().Format("2006-01-02"), time.UnixMilli(s.UntilMs).Local().Format("2006-01-02"))
	}
	where := ""
	if s.Projects > 1 {
		where = fmt.Sprintf(" across %d projects", s.Projects)
	}
	fmt.Fprintf(c.ErrWriter, "\n%s in %s%s%s\n", plural(int(s.Hits), "hit"), plural(int(s.Sessions), "conversation"), where, window)
	if s.Examined > 0 {
		fmt.Fprintf(c.ErrWriter, "%s read\n", plural(int(s.Examined), "message"))
	}
	if s.Pending > 0 {
		// An answer from an index that is behind is incomplete, and saying so
		// is the difference between that and an answer that is wrong.
		fmt.Fprintf(c.ErrWriter, "%s not yet indexed; run the search again, or --progress to see them\n", plural(int(s.Pending), "conversation"))
	}
	if s.Unavailable > 0 {
		fmt.Fprintf(c.ErrWriter, "%d projects could not be read; their conversations were not searched\n", s.Unavailable)
	}
	if s.Truncated > 0 {
		fmt.Fprintf(c.ErrWriter, "%d conversations have had older events trimmed by the size limit\n", s.Truncated)
	}
	if s.HasMore && s.NextCursor != "" {
		fmt.Fprintf(c.ErrWriter, "\nmore to read in this window: cxz conversation search --continue %s\n", s.NextCursor)
		return nil
	}
	// With the window exhausted, the useful next step is the one before it.
	if s.SinceMs > 0 && s.UntilMs > 0 {
		since := time.UnixMilli(s.SinceMs)
		span := time.UnixMilli(s.UntilMs).Sub(since)
		fmt.Fprintf(c.ErrWriter, "\nolder: cxz conversation search --until %s --window %s %s\n",
			since.UTC().Format(time.RFC3339), spanString(span), shellQuote(query))
	}
	return nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// spanString writes a span the way the flag that produced it reads, so the
// command a summary suggests is a command a person would have typed.
func spanString(d time.Duration) string {
	if d%(24*time.Hour) == 0 {
		return strconv.Itoa(int(d/(24*time.Hour))) + "d"
	}
	return d.String()
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool { return r == '\'' || r == ' ' || r == '"' || r == '$' || r == '`' }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// resolveProjects turns what a person typed into runtime ids. An id, an alias
// and a name are all acceptable; something that matches none of them is a
// mistake worth stopping for, because silently searching everything is the
// opposite of what --project asked.
func resolveProjects(known []*api.Project, named []string) ([]string, error) {
	var out []string
	for _, want := range named {
		found := ""
		for _, p := range known {
			if p.Id == want || p.Alias == want || p.Name == want || p.Workspace == want {
				if found != "" && found != p.Id {
					return nil, fmt.Errorf("project %q is ambiguous; use its id", want)
				}
				found = p.Id
			}
		}
		if found == "" {
			return nil, fmt.Errorf("unknown project %q; use cxz project ls", want)
		}
		out = append(out, found)
	}
	return out, nil
}

// parseWhen accepts the three ways a person says when: an instant, a day, or
// how long ago.
func parseWhen(v string, now time.Time) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.UTC(), nil
	}
	if d, err := parseSpan(strings.TrimSuffix(v, " ago")); err == nil {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339, YYYY-MM-DD, or a duration such as 90d ago")
}

// parseSpan is time.ParseDuration plus the units a month-at-a-time search is
// actually asked in. Go stops at hours; nobody asks for 720h.
func parseSpan(v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if n, ok := spanUnit(v, "d"); ok {
		return time.Duration(n) * 24 * time.Hour, nil
	}
	if n, ok := spanUnit(v, "w"); ok {
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	}
	if n, ok := spanUnit(v, "mo"); ok {
		return time.Duration(n) * searchWindow, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("expected a duration such as 36h, 14d, 2w or 3mo")
	}
	if d <= 0 {
		return 0, fmt.Errorf("a span must be positive")
	}
	return d, nil
}

func spanUnit(v, unit string) (int, bool) {
	if !strings.HasSuffix(v, unit) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(v, unit))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
