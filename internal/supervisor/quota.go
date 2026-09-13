package supervisor

import "time"

// Called under the supervisor lock. Keep telemetry failures independent of
// conversation state, and do not flood a provider that has not answered yet.
func (s *Supervisor) requestQuota(request any) {
	now := time.Now()
	if s.quotaDisabled {
		return
	}
	if !s.quotaRequested.IsZero() {
		if now.Sub(s.quotaRequested) < time.Minute {
			return
		}
		s.event("usage_status", "timeout", "", nil, nil)
	}
	s.quotaRequested = now
	s.event("usage_status", "polling", "", nil, nil)
	if err := s.write(request); err != nil {
		s.quotaRequested = time.Time{}
		s.event("usage_status", "error", "", nil, nil)
	}
}
