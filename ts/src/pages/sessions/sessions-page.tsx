import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { ResizeHandle } from "@lesomnus/nox";
import { usePaneWidth } from "#src/shared/layout/use-pane-width.ts";
import { useQuery } from "@lesomnus/payday/react";
import { SessionService } from "#gen/cxz/session_svc_pb";
import type { Project } from "#gen/cxz/project_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { Conversation } from "#src/features/session/conversation/conversation.tsx";
import { ProjectEditorPane } from "#src/features/workspace/editor/project-editor-pane.tsx";
import { SessionLibrary } from "#src/features/session/components/session-library.tsx";
import { MarkdownFileLinks } from "#src/shared/content/markdown.tsx";
import { useSession } from "#src/shared/api/use-session.ts";
import { useResourceInventory } from "#src/shared/api/resource-inventory.ts";
import { ConversationLoading } from "#src/features/session/conversation/conversation-loading.tsx";
import { Button } from "@lesomnus/nox";
import { useNavigate } from "@tanstack/react-router";
import {
  sessionLocation,
  uuid,
} from "#src/shared/navigation/resource-route.ts";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { createFileSizeReader } from "#src/shared/content/file-size.ts";
import { projectId } from "#src/shared/api/project-id.ts";
export function SessionsPage({
  c,
  id,
  projects,
}: {
  c: Connection;
  id: string;
  projects: Project[];
}) {
  useLocale();
  const inventory = useResourceInventory(c);
  const resolved = useSession(c, inventory.sessions, id);
  const navigate = useNavigate();
  const { seq } = useWorkspaceRoute();
  useEffect(() => {
    if (resolved.session?.alias && resolved.session.alias !== id && !uuid(id))
      void navigate({
        ...sessionLocation(resolved.session),
        search: { seq },
        replace: true,
      });
  }, [navigate, resolved.session?.alias, id, seq]);
  return id ? (
    resolved.session ? (
      <SessionWorkspace
        c={c}
        id={resolved.session.runtimeId}
        projects={projects}
      />
    ) : (
      <div className="session-workspace">
        {resolved.error ? (
          <div role="alert">
            <p>{resolved.error}</p>
            <Button onClick={resolved.reload}>Retry</Button>
          </div>
        ) : (
          <ConversationLoading stage="session" />
        )}
      </div>
    )
  ) : (
    <SessionLibrary c={c} projects={projects} />
  );
}
function SessionWorkspace({
  c,
  id,
  projects,
}: {
  c: Connection;
  id: string;
  projects: Project[];
}) {
  useLocale();
  const current = useQuery(SessionService.method.get, {
    ref: ref(id),
    select: { all: true, project: { all: true } },
  });
  const area = useRef<HTMLDivElement>(null);
  const pane = usePaneWidth(area, "session");
  const [activated, setActivated] = useState(false);
  const [wide, setWide] = useState(false);
  const [fileRequest, setFileRequest] = useState<{
    session: string;
    path: string;
  }>();
  const requestedFile = fileRequest?.session === id ? fileRequest : undefined;
  const openFile = useCallback(
    (path: string) => {
      setActivated(true);
      setFileRequest({ session: id, path });
    },
    [id],
  );
  useEffect(() => {
    const observer = new ResizeObserver(([entry]) => {
      const wide = entry.contentRect.width >= 1600;
      setWide(wide);
      if (wide) setActivated(true);
    });
    observer.observe(area.current!);
    return () => observer.disconnect();
  }, []);
  const project = current.data?.project;
  const projectKey = project?.id.length ? projectId(project) : "";
  const readSize = useMemo(
    () =>
      projectKey ? createFileSizeReader(c.projects, projectKey) : undefined,
    [c, projectKey],
  );
  const fileLinks = useMemo(
    () => ({ open: openFile, size: readSize }),
    [openFile, readSize],
  );
  return (
    <MarkdownFileLinks.Provider value={fileLinks}>
      <div
        className="session-workspace"
        ref={area}
        data-file-open={!!requestedFile}
      >
        <div
          className="session-split"
          style={
            { "--session-resized-width": `${pane.value}px` } as CSSProperties
          }
        >
          <Conversation key={id} c={c} id={id} projects={projects} />
          {activated && !!project?.id.length && (
            <>
              <ResizeHandle
                {...pane}
                className="session-divider"
                label={t("Resize workspace editor")}
              />
              <div className="workspace-editor-layer">
                <ProjectEditorPane
                  key={Array.from(project.id).join("-")}
                  c={c}
                  projectId={project.id}
                  requestedFile={requestedFile}
                  closePreview={
                    requestedFile && !wide
                      ? () => setFileRequest(undefined)
                      : undefined
                  }
                />
              </div>
            </>
          )}
        </div>
      </div>
    </MarkdownFileLinks.Provider>
  );
}
