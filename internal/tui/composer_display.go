package tui

func (m *model) composerDisplay(view string) string {
	m.prepareEditor()
	return m.input.RenderDisplay(view)
}
