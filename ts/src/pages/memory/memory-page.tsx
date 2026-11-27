import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useCallback, useEffect, useMemo } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Section } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { MemoryGrid } from "#src/features/memory/memory-grid.tsx";
import { useProjectMemories } from "#src/features/memory/memory-catalog.ts";
import { ResourceSearch } from "#src/shared/components/resource-search.tsx";
import { useResourceSearch } from "#src/shared/components/use-resource-search.ts";
import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import { Icon } from "#src/shared/icons/icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import searchStyles from "#src/shared/components/resource-search.module.css";
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
  resourceHandle,
  uuid,
  useResourceRoutes,
} from "#src/shared/navigation/resource-route.ts";
import styles from "./memory-page.module.css";
import { ListDetailPages } from "@lesomnus/nox";

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
  const { memoryLocation } = useResourceRoutes();
  const navigate = useNavigate();
  const { memoryProject, memory } = useWorkspaceRoute();
  const selection = useResourceSelection(c);
  const project = memoryProject
    ? projects.find((p) => matchesResource(p, memoryProject))
    : (projects.find((p) => projectId(p) === selection.project) ?? projects[0]);
  const id = projectId(project) ?? "";
  const handle = project ? resourceHandle(project) : "";
  const detailOpen = !!project && !!memory;
  useEffect(() => {
    if (id) c.selection.chooseProject(id);
  }, [c, id]);
  const selectMemory = useCallback(
    (selected?: string, replace = false) => {
      if (handle && (memoryProject !== handle || memory !== selected)) {
        void navigate({
          ...memoryLocation(handle, selected),
          replace,
        });
      }
    },
    [navigate, handle, memoryProject, memory],
  );
  const scoped = sessions.filter(
    (s) => s.project && projectId(s.project) === id,
  );
  const sessionFilter =
    selection.project === id ? uuid(selection.session) : undefined;
  const search = useResourceSearch();
  const sessionId = sessionUUID(sessionFilter);
  const catalog = useProjectMemories(
    c,
    id,
    !detailOpen || search.open,
    sessionId,
  );
  const searchItems = useMemo(
    () =>
      catalog.memories
        .filter((memory) => !sessionId || memory.ownerSession === sessionId)
        .map((memory) => ({
          id: resourceHandle(memory),
          title: memory.name || memory.alias,
          text: [
            memory.name,
            memory.alias,
            memory.desc,
            ...Object.entries(memory.labels)
              .filter(([key]) => !key.startsWith("legacy."))
              .map(([key, value]) => `${key}=${value}`),
          ]
            .filter(Boolean)
            .join("\n"),
          date: memory.dateUpdated ?? memory.dateCreated,
          icon: <Icon name="document" aria-hidden="true" />,
        })),
    [catalog.memories, sessionId],
  );
  return (
    <ResourcePage
      ref={search.area}
      className={searchStyles.page}
      aria-label="Shared memory workspace"
    >
      <ResourceToolbar
        actions={
          <ToolbarIconButton
            ref={search.trigger}
            icon="search"
            label={t("Search memories")}
            disabled={!project}
            aria-pressed={search.open}
            aria-keyshortcuts="Shift+F Control+Shift+F Meta+Shift+F"
            onClick={search.show}
          />
        }
      >
        <ProjectPicker
          projects={projects}
          value={id}
          loading={projectsLoading}
          choose={(value) => {
            const chosen = projects.find((p) => projectId(p) === value)!;
            c.selection.chooseProject(value);
            void navigate(memoryLocation(chosen));
          }}
        />
        <SessionPicker
          sessions={scoped}
          project={id}
          value={sessionUUID(sessionFilter)}
          loading={sessionsLoading}
          choose={(value) => {
            const session = scoped.find((s) => sessionUUID(s.id) === value);
            c.selection.chooseSession(
              id,
              sessionUUID(session?.id),
              session?.runtimeId ?? "",
            );
            if (memory) void navigate(memoryLocation(handle));
          }}
        />
      </ResourceToolbar>
      <ListDetailPages
        key={id}
        scope={c}
        open={detailOpen}
        list={
          <PanelScroll
            contentClassName={pageStyles.contentPadding}
            scrollbarLabel="Shared memory scroll"
          >
            <ResourceLayout>
              {error && <ErrorNotice className={styles.error} error={error} />}
              {project ? (
                <MemoryGrid
                  key={id}
                  list={catalog}
                  projectHandle={handle}
                  sessions={scoped}
                  session={sessionFilter}
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
                  projectInfo={project}
                  catalog={false}
                  sessions={scoped}
                  memoryId={memory}
                  onMemoryChange={selectMemory}
                />
              </ResourceLayout>
            </PanelScroll>
          ) : null
        }
      />
      <ResourceSearch
        search={search}
        items={searchItems}
        label={t("Search memories")}
        loading={catalog.loading}
        error={catalog.error}
        select={(id) => selectMemory(id, detailOpen)}
      />
    </ResourcePage>
  );
}
