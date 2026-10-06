package supervisor

import "github.com/lesomnus/cxz/internal/core"

func (s *Supervisor) claudeResponseContext() core.ResponseMetadata {
	if s.appliedModel != "" {
		metadata := core.ResponseMetadata{Model: s.appliedModel, ModelSource: "settings", Effort: s.appliedEffort}
		if metadata.Effort != "" {
			metadata.EffortSource = "settings"
		}
		return metadata
	}
	metadata := core.ResponseMetadata{Model: s.session.Model, Effort: s.effort}
	if metadata.Model != "" {
		metadata.ModelSource = "requested"
	}
	if metadata.Effort != "" {
		metadata.EffortSource = "requested"
	}
	return metadata
}
