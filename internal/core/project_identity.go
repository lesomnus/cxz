package core

import (
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/projectid"
)

func (s *Session) UnmarshalJSON(data []byte) error {
	type record Session
	var v record
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if projectid.Legacy(v.ProjectID) {
		v.LegacyProjectID = v.ProjectID
		v.ProjectID = projectid.Canonical(v.ProjectID)
	}
	if v.LegacyProjectID != "" && (!projectid.Legacy(v.LegacyProjectID) || !projectid.Same(v.ProjectID, v.LegacyProjectID)) {
		return fmt.Errorf("session legacy project identity mismatch")
	}
	*s = Session(v)
	return nil
}

// Existing bindings were derived from the old handle. Keep that authentication
// input while every other session operation uses the canonical project UUID.
func (s Session) AuthProjectID() string {
	if s.LegacyProjectID != "" {
		return s.LegacyProjectID
	}
	return s.ProjectID
}
