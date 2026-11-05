import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useBlocker } from "@tanstack/react-router";
import { PageToolbar, Section } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { MemoryGrid } from "#src/features/memory/memory-grid.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { MemoryLibrary } from "#src/features/memory/memory-library.tsx";
import { ProjectPicker } from "#src/shared/components/project-picker.tsx";
import { SessionPicker } from "#src/shared/components/session-picker.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { sessionUUID } from "#src/shared/api/session-id.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import {
  matchesResource,
  memoryLocation,
  resourceHandle,
} from "#src/shared/navigation/resource-route.ts";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import styles from "./memory-page.module.css";

export function MemoryPage({
  c,
  projects,
  sessions,
  projectsLoading = false,
  sessionsLoading = false,
  error,
}: {
  c: Connection;
  projects: Project[];
  sessions: Session[];
  projectsLoading?: boolean;
  sessionsLoading?: boolean;
  error?: string;
}) {
  const navigate = useNavigate();
  const { memoryProject, memory } = useWorkspaceRoute();
  const [dirty, setDirty] = useState(false);
  const [creatingProject, setCreatingProject] = useState<string>();
  const [sessionSelection, setSessionSelection] = useState<{
    project: string;
    id: Uint8Array;
  }>();
  const dirtyRef = useRef(false);
  const onDirtyChange = useCallback((value: boolean) => {
    dirtyRef.current = value;
    setDirty(value);
  }, []);
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) =>
      dirtyRef.current && current.pathname !== next.pathname,
    enableBeforeUnload: dirty,
    withResolver: true,
  });
  const project = memoryProject
    ? projects.find((p) => matchesResource(p, memoryProject))
    : projects[0];
  const id = projectId(project) ?? "";
  const handle = project ? resourceHandle(project) : "";
  useEffect(() => {
    setCreatingProject(undefined);
    setSessionSelection(undefined);
  }, [id]);
  const onCreatingChange = useCallback(
    (creating: boolean) => setCreatingProject(creating ? id : undefined),
    [id],
  );
  const selectMemory = useCallback(
    (selected?: string, replace = false) => {
      if (handle && (memoryProject !== handle || memory !== selected)) {
        const navigation = navigate({
          ...memoryLocation(handle, selected),
          replace,
          ignoreBlocker: true,
        });
        if (selected) void navigation.then(() => setCreatingProject(undefined));
      }
    },
    [navigate, handle, memoryProject, memory],
  );
  const scoped = sessions.filter(
    (s) => s.project && projectId(s.project) === id,
  );
  const sessionFilter =
    sessionSelection?.project === id ? sessionSelection.id : undefined;
  const guard = useRef<(run: () => void) => void>((run) => run());
  const registerGuard = useCallback(
    (next: ((run: () => void) => void) | undefined) => {
      guard.current = next ?? ((run) => run());
    },
    [],
  );
  return (
    <main className={styles.page} aria-label="Shared memory workspace">
      <PageToolbar className={styles.toolbar}>
        <div className={styles.toolbarContent}>
          {memory && (
            <RouteLink
              {...memoryLocation(handle || memoryProject!)}
              variant="toolbar"
              className={styles.back}
              aria-label="Back to memories"
              title="Back to memories"
            >
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.6"
                aria-hidden="true"
              >
                <path d="m14 5-7 7 7 7M7 12h13" />
              </svg>
            </RouteLink>
          )}
          <ProjectPicker
            projects={projects}
            value={id}
            loading={projectsLoading}
            choose={(value) =>
              guard.current(() => {
                const chosen = projects.find((p) => projectId(p) === value)!;
                void navigate({
                  ...memoryLocation(chosen),
                  ignoreBlocker: true,
                });
              })
            }
          />
          <SessionPicker
            sessions={scoped}
            project={id}
            value={sessionUUID(sessionFilter)}
            loading={sessionsLoading}
            choose={(value) =>
              guard.current(() => {
                const session = scoped.find((s) => sessionUUID(s.id) === value);
                setSessionSelection(
                  session ? { project: id, id: session.id } : undefined,
                );
                if (memory || creatingProject === id) {
                  setCreatingProject(undefined);
                  void navigate({
                    ...memoryLocation(handle),
                    ignoreBlocker: true,
                  });
                }
              })
            }
          />
        </div>
      </PageToolbar>
      <PanelScroll
        className={styles.scroll}
        contentClassName={styles.scrollContent}
        scrollbarLabel="Shared memory scroll"
      >
        <div className={styles.content}>
          {error && (
            <p className={styles.error} role="alert">
              {error}
            </p>
          )}
          {project ? (
            memory || creatingProject === id ? (
              <MemoryLibrary
                key={id}
                c={c}
                project={id}
                projectHandle={handle}
                catalog={false}
                sessions={scoped}
                registerGuard={registerGuard}
                memoryId={memory}
                onMemoryChange={selectMemory}
                onDirtyChange={onDirtyChange}
                onCreatingChange={onCreatingChange}
              />
            ) : (
              <MemoryGrid
                key={id}
                c={c}
                project={id}
                projectHandle={handle}
                sessions={scoped}
                session={sessionFilter}
              />
            )
          ) : (
            <Section className={styles.empty}>
              <strong>
                {projectsLoading || sessionsLoading
                  ? "Loading shared memory…"
                  : !project
                    ? memoryProject
                      ? "Project unavailable"
                      : "No projects yet"
                    : "Project unavailable"}
              </strong>
              {!projectsLoading && !sessionsLoading && (
                <p>
                  {memoryProject
                    ? "This project is not available on this connection."
                    : "Add a project to start sharing memory documents."}
                </p>
              )}
            </Section>
          )}
        </div>
      </PanelScroll>
      {blocker.status === "blocked" && (
        <ConfirmationDialog
          title="Discard unsaved changes"
          confirmLabel="Confirm"
          close={() => blocker.reset?.()}
          execute={() => {
            blocker.proceed?.();
            return true;
          }}
        >
          <p>This memory has unsaved changes. Discard them?</p>
        </ConfirmationDialog>
      )}
    </main>
  );
}
