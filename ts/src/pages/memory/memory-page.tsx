import { useCallback, useRef } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Button, PageToolbar, Section } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { MemoryLibrary } from "#src/features/memory/memory-library.tsx";
import { ProjectPicker } from "#src/shared/components/project-picker.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { projectId } from "#src/shared/api/project-id.ts";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import {
  matchesResource,
  resourceHandle,
  resourceLocation,
  projectLocation,
} from "#src/shared/navigation/resource-route.ts";
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
  const { memoryProject } = useWorkspaceRoute();
  const project = memoryProject
    ? projects.find((p) => matchesResource(p, memoryProject))
    : projects[0];
  const id = projectId(project) ?? "";
  const scoped = sessions.filter(
    (s) => s.project && projectId(s.project) === id,
  );
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
          <ProjectPicker
            projects={projects}
            value={id}
            loading={projectsLoading}
            choose={(value) =>
              guard.current(() => {
                const chosen = projects.find((p) => projectId(p) === value)!;
                void navigate({
                  ...resourceLocation("memory"),
                  search: { project: resourceHandle(chosen) },
                });
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
            <MemoryLibrary
              key={id}
              c={c}
              project={id}
              sessions={scoped}
              registerGuard={registerGuard}
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
              {!projectsLoading && !sessionsLoading && project && (
                <>
                  <p>Choose a project to browse its memory documents.</p>
                  <Button
                    onClick={() =>
                      void navigate(projectLocation(resourceHandle(project)))
                    }
                  >
                    Open project
                  </Button>
                </>
              )}
            </Section>
          )}
        </div>
      </PanelScroll>
    </main>
  );
}
