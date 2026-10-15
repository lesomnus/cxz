import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useEffect, useRef, useState, type CSSProperties } from "react";
import { ResizeHandle } from "@lesomnus/nox";
import { usePaneWidth } from "#src/shared/layout/use-pane-width.ts";
import { useQuery } from "@lesomnus/payday/react";
import { SessionService } from "#gen/cxz/session_svc_pb";
import type { Project } from "#gen/cxz/project_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { Conversation } from "#src/features/session/conversation/conversation.tsx";
import { ProjectEditorPane } from "#src/features/workspace/editor/project-editor-pane.tsx";
import { SessionLibrary } from "#src/features/session/components/session-library.tsx";
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
  return id ? (
    <SessionWorkspace c={c} id={id} projects={projects} />
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
  useEffect(() => {
    const observer = new ResizeObserver(([entry]) => {
      if (entry.contentRect.width >= 1600) setActivated(true);
    });
    observer.observe(area.current!);
    return () => observer.disconnect();
  }, []);
  const project = current.data?.project;
  return (
    <div className="session-workspace" ref={area}>
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
            <ProjectEditorPane
              key={Array.from(project.id).join("-")}
              c={c}
              projectId={project.id}
            />
          </>
        )}
      </div>
    </div>
  );
}
