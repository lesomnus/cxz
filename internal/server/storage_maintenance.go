package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/editor"
	"github.com/lesomnus/cxz/internal/maintenance"
	"github.com/lesomnus/cxz/internal/memorylib"
	"github.com/lesomnus/cxz/internal/sessionstore"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/lifecycle"
)

func (s *Server) maintenanceCollector() maintenance.Collect {
	versions := func(ctx context.Context, p maintenance.Policy, now time.Time, pressure bool) ([]maintenance.Candidate, error) {
		refs := func(ctx context.Context) (maintenance.References, error) {
			return maintenance.ReadReferences(ctx, []string{s.root}, "/cxz/tools")
		}
		return maintenance.VersionCandidates(ctx, []string{filepath.Join(s.root, "runtime-binaries")}, maintenance.VersionLock(s.root), refs, p, now, pressure)
	}
	residue := maintenance.LegacyJournals(s.root, sessionstore.RemoveConvertedJournal)
	staging := func(_ context.Context, p maintenance.Policy, now time.Time, pressure bool) ([]maintenance.Candidate, error) {
		return maintenance.StagingCandidates([]string{filepath.Join(s.root, "runtime-binaries", "*", "*", ".pin-*")}, maintenance.VersionLock(s.root), p, now, pressure)
	}
	collectors := []maintenance.Collect{maintenance.LocalFiles(s.root), residue, staging, versions}
	if os.Getenv("CXZ_PROJECT_ID") != "" {
		if path, err := editor.LogPath(); err == nil {
			collectors = append(collectors, func(_ context.Context, p maintenance.Policy, _ time.Time, pressure bool) ([]maintenance.Candidate, error) {
				return maintenance.ComponentLogCandidates(path, p, pressure)
			})
		}
	}
	if s.manager != nil {
		collectors = append(collectors, s.manager.MaintenanceVersions, s.manager.MaintenanceFiles)
	}
	return maintenance.Combine(collectors...)
}

func (s *Server) startMaintenance(ctx context.Context, resources resource.Server) func() {
	if path, err := cxzupdate.PinSelf(s.root); err == nil {
		_ = core.WriteJSON(filepath.Join(s.root, "run", "maintenance-binary.json"), map[string]string{"binary": path, "revision": cxzupdate.Current().Revision})
	}
	roots := []string{s.root}
	if os.Getenv("CXZ_PROJECT_ID") != "" {
		if path, err := editor.LogPath(); err == nil {
			// The home filesystem exists before the editor is first opened.
			roots = append(roots, filepath.Dir(filepath.Dir(filepath.Dir(path))))
		}
	}
	if s.manager != nil {
		if st, err := os.Stat("/cxz/tools"); err == nil && st.IsDir() {
			roots = append(roots, "/cxz/tools")
		}
	}
	r := maintenance.NewRunner(s.root, maintenance.Engine{Roots: roots, Collect: s.maintenanceCollector()})
	r.Error = func(err error) { fmt.Fprintln(os.Stderr, "storage maintenance:", err) }
	if s.manager != nil {
		r.Configure = func(ctx context.Context, _ maintenance.Policy) error { return s.manager.SyncMaintenancePolicy(ctx) }
	}
	layer, _ := resource.Find[lifecycle.Layer](resources)
	r.Retain = func(ctx context.Context, p maintenance.Policy, now time.Time) error {
		var errs []error
		if _, err := layer.RetainAudit(ctx, now, p.AuditDays); err != nil {
			errs = append(errs, err)
		}
		if _, err := layer.RetainNotifications(ctx, now, p.NotificationDays); err != nil {
			errs = append(errs, err)
		}
		if _, err := memorylib.SweepTrash(ctx, s.root, now, p.TrashDays); err != nil {
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	}
	s.maintenance = r
	work, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); r.Run(work) }()
	return func() { cancel(); <-done }
}
