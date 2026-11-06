package workspace

func (m *Manager) Close() {
	if m.auxStop != nil {
		m.auxStop()
		<-m.auxDone
	}
	m.auxMu.Lock()
	if m.aux != nil {
		m.aux.Close()
	}
	m.auxMu.Unlock()
	m.paths.Close()
}
