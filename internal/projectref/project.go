// Package projectref defines the same project handles for every CLI workflow.
package projectref

import (
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
)

// Resolve prefers exact identity/path, then a normalized alias, then display
// name. Display names need not be unique. Foreign containers are never targets.
func Resolve(projects []*api.Project, handle string) (*api.Project, error) {
	for _, p := range projects {
		if p.State != "foreign" && (p.Id == handle || p.Workspace == handle) {
			return p, nil
		}
	}
	for _, p := range projects {
		if p.State != "foreign" && p.Alias != "" && p.Alias == strings.ToLower(strings.TrimSpace(handle)) {
			return p, nil
		}
	}
	var found *api.Project
	for _, p := range projects {
		if p.State != "foreign" && p.Name == handle {
			if found != nil && found.Id != p.Id {
				return nil, status.Error(codes.InvalidArgument, "ambiguous project display name; use its alias, ID or path")
			}
			found = p
		}
	}
	if found != nil {
		return found, nil
	}
	return nil, status.Error(codes.NotFound, "project not found: "+handle)
}

// ShortAlias follows cld's short-name/initials/truncation convention, restricted
// to payday's lowercase ASCII slug grammar. Uniqueness belongs to the server.
func ShortAlias(name string) string {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') })
	s := strings.Join(parts, "-")
	if s == "" {
		return "p"
	}
	if len(s) > 6 {
		if len(parts) > 1 {
			s = ""
			for _, p := range parts {
				s += p[:1]
			}
		}
		if len(s) > 6 {
			s = s[:6]
		}
	}
	if s[0] < 'a' || s[0] > 'z' {
		s = "p-" + s
	}
	return strings.TrimRight(s, "-")
}
