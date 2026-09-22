package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

var codeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("238"))
var markdownParser = goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()

type renderedResponse struct {
	source, agent, body string
	width               int
	buttons             []codeButton
}

func eventViewCached(m *model, s *api.Session, e *api.Event, width int) string {
	if e.Kind != "assistant" {
		return eventView(s, e, width)
	}
	if cached, ok := m.renderedResponses[e]; ok && cached.source == e.Text && cached.agent == s.Agent && cached.width == width {
		return cached.body
	}
	if m.renderedResponses == nil {
		m.renderedResponses = map[*api.Event]renderedResponse{}
	}
	response := renderResponse(s.Agent, e.Text, width)
	m.renderedResponses[e] = response
	return response.body
}

// Protocol text has no Markdown MIME hint. Parse CommonMark when structural
// syntax is present. Never render HTML, OSC links or fetch external images.
func markdownView(raw string, width int) string {
	body, _ := markdownContent(raw, width)
	return body
}

func renderResponse(agent, raw string, width int) renderedResponse {
	body, buttons := markdownContent(raw, max(1, width-2))
	name := strings.ToUpper(pickerLabel(safeText(agent)))
	style := lavender.Bold(true)
	switch agent {
	case "claude":
		style = claude
	case "codex":
		style = codex
	}
	if name == "" {
		name = "AGENT"
	}
	for i := range buttons {
		buttons[i].x += 2
		buttons[i].y++
	}
	return renderedResponse{source: raw, agent: agent, width: width, body: style.Render("•") + " " + style.Render(name) + "\n" + indentBlock(body), buttons: buttons}
}

// Internal zero-width markers survive ANSI-aware wrapping through nested lists
// and quotes. Remove them before caching/display; remote control sequences have
// already been stripped. Copy hitboxes therefore follow the actual layout.
var codeButtonMarker = regexp.MustCompile("\x1b]cxz-copy;([0-9]+)\x07")

func markdownContent(raw string, width int) (string, []codeButton) {
	source := []byte(safeText(raw))
	doc := markdownParser.Parse(text.NewReader(source))
	formatted := false
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch n.(type) {
			case *ast.Heading, *ast.List, *ast.CodeSpan, *ast.FencedCodeBlock, *ast.CodeBlock, *ast.Emphasis, *ast.Link, *ast.Blockquote, *extast.Table, *extast.Strikethrough:
				formatted = true
			}
		}
		return ast.WalkContinue, nil
	})
	if !formatted {
		return answer.Render(ansi.Hardwrap(string(source), max(1, width), true)), nil
	}
	var render func(ast.Node, int) string
	children := func(n ast.Node, width int) string {
		var b strings.Builder
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			b.WriteString(render(c, width))
		}
		return b.String()
	}
	var sources []string
	blockCode := func(n ast.Node, width int) string {
		var b strings.Builder
		for i := 0; i < n.Lines().Len(); i++ {
			segment := n.Lines().At(i)
			b.Write(segment.Value(source))
		}
		language := ""
		if fence, ok := n.(*ast.FencedCodeBlock); ok {
			language = string(fence.Language(source))
		}
		code := highlightCode(strings.TrimRight(b.String(), "\n"), language)
		rows := strings.Split(ansi.Hardwrap(code, max(1, width-1), true), "\n")
		for i := range rows {
			line := clip(" "+rows[i], max(1, width))
			rows[i] = indexedBackground(line+strings.Repeat(" ", max(0, width-ansi.StringWidth(line))), 238)
		}
		id := len(sources)
		sources = append(sources, b.String())
		header := strings.Repeat(" ", max(0, width-3)) + fmt.Sprintf("\x1b]cxz-copy;%d\a", id) + " ⧉ "
		rows = append([]string{indexedBackground(header, 238)}, rows...)
		rows = append(rows, indexedBackground(strings.Repeat(" ", max(1, width)), 238))
		return strings.Join(rows, "\n") + "\n\n"
	}
	render = func(n ast.Node, width int) string {
		switch v := n.(type) {
		case *ast.Text:
			s := string(v.Segment.Value(source))
			if v.SoftLineBreak() || v.HardLineBreak() {
				s += "\n"
			}
			return s
		case *ast.String:
			return string(v.Value)
		case *ast.CodeSpan:
			return codeStyle.Render(strings.ReplaceAll(string(v.Text(source)), "\n", " "))
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			return blockCode(n, width)
		case *ast.Heading:
			return strong.Render(children(n, width)) + "\n\n"
		case *ast.Emphasis:
			if v.Level == 2 {
				return strong.Render(children(n, width))
			}
			return lipgloss.NewStyle().Italic(true).Render(children(n, width))
		case *ast.Link:
			return children(n, width) + " (" + safeText(string(v.Destination)) + ")"
		case *ast.Image:
			return "[image: " + children(n, width) + "]" // No remote fetch.
		case *ast.AutoLink:
			return string(v.URL(source))
		case *extast.Strikethrough:
			return lipgloss.NewStyle().Strikethrough(true).Render(children(n, width))
		case *extast.TaskCheckBox:
			if v.IsChecked {
				return "☑ "
			}
			return "☐ "
		case *extast.Table:
			var rows [][]string
			for row := n.FirstChild(); row != nil; row = row.NextSibling() {
				var cells []string
				for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
					cells = append(cells, children(cell, width))
				}
				rows = append(rows, cells)
			}
			return markdownTable(rows, v.Alignments, width) + "\n\n"
		case *ast.ListItem:
			prefix := "• "
			if list, ok := n.Parent().(*ast.List); ok && list.IsOrdered() {
				i := list.Start
				for c := list.FirstChild(); c != n; c = c.NextSibling() {
					i++
				}
				prefix = fmt.Sprintf("%d. ", i)
			}
			indent := strings.Repeat(" ", ansi.StringWidth(prefix))
			bodyWidth := max(1, width-ansi.StringWidth(prefix))
			content := strings.TrimRight(children(n, bodyWidth), "\n")
			content = ansi.Hardwrap(content, bodyWidth, true)
			return prefix + strings.ReplaceAll(content, "\n", "\n"+indent) + "\n"
		case *ast.List:
			return children(n, width) + "\n"
		case *ast.Blockquote:
			bodyWidth := max(1, width-2)
			content := ansi.Hardwrap(strings.TrimRight(children(n, bodyWidth), "\n"), bodyWidth, true)
			return "│ " + strings.ReplaceAll(content, "\n", "\n│ ") + "\n\n"
		case *ast.ThematicBreak:
			return muted.Render(strings.Repeat("─", min(20, max(0, width-2)))) + "\n\n"
		case *ast.Paragraph:
			return children(n, width) + "\n\n"
		case *ast.TextBlock:
			return children(n, width) + "\n"
		case *ast.HTMLBlock, *ast.RawHTML:
			return "" // Never interpret embedded terminal/HTML instructions.
		default:
			return children(n, width)
		}
	}
	view := answer.Render(ansi.Hardwrap(strings.TrimRight(render(doc, max(1, width)), "\n"), max(1, width), true))
	if len(sources) == 0 {
		return view, nil
	}
	rows := strings.Split(view, "\n")
	var buttons []codeButton
	for y, row := range rows {
		for {
			loc := codeButtonMarker.FindStringSubmatchIndex(row)
			if loc == nil {
				break
			}
			id, _ := strconv.Atoi(row[loc[2]:loc[3]])
			x := ansi.StringWidth(row[:loc[0]])
			if id < len(sources) && x+3 <= width {
				buttons = append(buttons, codeButton{x: x, y: y, source: sources[id]})
			}
			row = row[:loc[0]] + row[loc[1]:]
		}
		rows[y] = row
	}
	return strings.Join(rows, "\n"), buttons
}
