package tui

import (
	"fmt"
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

var codeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("238"))
var markdownParser = goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()

type renderedResponse struct {
	source, agent, body string
	width               int
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
	body := eventView(s, e, width)
	m.renderedResponses[e] = renderedResponse{source: e.Text, agent: s.Agent, body: body, width: width}
	return body
}

// Protocol text has no Markdown MIME hint. Parse CommonMark when structural
// syntax is present. Never render HTML, OSC links or fetch external images.
func markdownView(raw string, width int) string {
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
		return answer.Render(ansi.Hardwrap(string(source), max(1, width), true))
	}
	var render func(ast.Node) string
	children := func(n ast.Node) string {
		var b strings.Builder
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			b.WriteString(render(c))
		}
		return b.String()
	}
	blockCode := func(n ast.Node) string {
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
		return strings.Join(rows, "\n") + "\n\n"
	}
	render = func(n ast.Node) string {
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
			return blockCode(n)
		case *ast.Heading:
			return strong.Render(children(n)) + "\n\n"
		case *ast.Emphasis:
			if v.Level == 2 {
				return strong.Render(children(n))
			}
			return lipgloss.NewStyle().Italic(true).Render(children(n))
		case *ast.Link:
			return children(n) + " (" + safeText(string(v.Destination)) + ")"
		case *ast.Image:
			return "[image: " + children(n) + "]" // No remote fetch.
		case *ast.AutoLink:
			return string(v.URL(source))
		case *extast.Strikethrough:
			return lipgloss.NewStyle().Strikethrough(true).Render(children(n))
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
					cells = append(cells, children(cell))
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
			content := strings.TrimRight(children(n), "\n")
			return prefix + strings.ReplaceAll(content, "\n", "\n"+strings.Repeat(" ", ansi.StringWidth(prefix))) + "\n"
		case *ast.List:
			return children(n) + "\n"
		case *ast.Blockquote:
			return "│ " + strings.ReplaceAll(strings.TrimRight(children(n), "\n"), "\n", "\n│ ") + "\n\n"
		case *ast.ThematicBreak:
			return muted.Render(strings.Repeat("─", min(20, max(0, width-2)))) + "\n\n"
		case *ast.Paragraph:
			return children(n) + "\n\n"
		case *ast.TextBlock:
			return children(n) + "\n"
		case *ast.HTMLBlock, *ast.RawHTML:
			return "" // Never interpret embedded terminal/HTML instructions.
		default:
			return children(n)
		}
	}
	return answer.Render(ansi.Hardwrap(strings.TrimRight(render(doc), "\n"), max(1, width), true))
}
