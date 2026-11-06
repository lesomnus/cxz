import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { useNavigate, useBlocker } from "@tanstack/react-router";
import { Section } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { MemoryGrid } from "#src/features/memory/memory-grid.tsx";
import { MemoryLibrary } from "#src/features/memory/memory-library.tsx";
import { ProjectPicker } from "#src/shared/components/project-picker.tsx";
import { SessionPicker } from "#src/shared/components/session-picker.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import {
  ResourceLayout,
  ResourcePage,
  ResourceToolbar,
} from "#src/shared/components/resource-layout.tsx";
import pageStyles from "#src/shared/components/resource-page.module.css";
import { sessionUUID } from "#src/shared/api/session-id.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { useResourceSelection } from "#src/shared/api/resource-selection.ts";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import {
  matchesResource,
  memoryLocation,
  resourceHandle,
  uuid,
} from "#src/shared/navigation/resource-route.ts";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import styles from "./memory-page.module.css";
import { ResourceDetailPages } from "#src/shared/components/resource-detail-pages.tsx";

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
  const selection = useResourceSelection(c);
  const dirtyRef = useRef(false);
  const detailActive = useRef(false);
  const onDirtyChange = useCallback((value: boolean) => {
    dirtyRef.current = value && detailActive.current;
    setDirty(dirtyRef.current);
  }, []);
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) =>
      dirtyRef.current && current.pathname !== next.pathname,
    enableBeforeUnload: dirty,
    withResolver: true,
  });
  const project = memoryProject
    ? projects.find((p) => matchesResource(p, memoryProject))
    : (projects.find((p) => projectId(p) === selection.project) ?? projects[0]);
  const id = projectId(project) ?? "";
  const handle = project ? resourceHandle(project) : "";
  const detailOpen = !!project && (!!memory || creatingProject === id);
  detailActive.current = detailOpen;
  useLayoutEffect(() => {
    if (!detailOpen) onDirtyChange(false);
  }, [detailOpen, onDirtyChange]);
  useEffect(() => {
    setCreatingProject(undefined);
    if (id) c.selection.chooseProject(id);
  }, [c, id]);
  const onCreatingChange = useCallback(
    (creating: boolean) => setCreatingProject(creating ? id : undefined),
    [id],
  );
  const selectMemory = useCallback(
    (selected?: string, replace = false) => {
      if (!detailActive.current) return;
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
    selection.project === id ? uuid(selection.session) : undefined;
  const guard = useRef<(run: () => void) => void>((run) => run());
  const registerGuard = useCallback(
    (next: ((run: () => void) => void) | undefined) => {
      guard.current = (run) => {
        if (next && detailActive.current) next(run);
        else run();
      };
    },
    [],
  );
  return (
    <ResourcePage aria-label="Shared memory workspace">
      <ResourceToolbar>
        <ProjectPicker
          projects={projects}
          value={id}
          loading={projectsLoading}
          choose={(value) =>
            guard.current(() => {
              const chosen = projects.find((p) => projectId(p) === value)!;
              c.selection.chooseProject(value);
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
              c.selection.chooseSession(
                id,
                sessionUUID(session?.id),
                session?.runtimeId ?? "",
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
      </ResourceToolbar>
      <ResourceDetailPages
        key={id}
        scope={c}
        open={detailOpen}
        list={
          <PanelScroll
            className={styles.scroll}
            contentClassName={pageStyles.contentPadding}
            scrollbarLabel="Shared memory scroll"
          >
            <ResourceLayout>
              {error && <ErrorNotice className={styles.error} error={error} />}
              {project ? (
                <MemoryGrid
                  key={id}
                  c={c}
                  project={id}
                  projectHandle={handle}
                  sessions={scoped}
                  session={sessionFilter}
                  active={!detailOpen}
                />
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
            </ResourceLayout>
          </PanelScroll>
        }
        detail={
          detailOpen ? (
            <PanelScroll
              className={styles.scroll}
              contentClassName={pageStyles.contentPadding}
              scrollbarLabel="Shared memory scroll"
            >
              <ResourceLayout>
                {error && (
                  <ErrorNotice className={styles.error} error={error} />
                )}
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
              </ResourceLayout>
            </PanelScroll>
          ) : null
        }
      />
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
    </ResourcePage>
  );
}
