import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useEffect, useRef, useState } from "react";
import { useQuery } from "@lesomnus/payday/react";
import { SessionService } from "#gen/cxz/session_svc_pb";
import type { Project } from "#gen/cxz/project_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { Conversation } from "#src/features/session/conversation/conversation.tsx";
import { ProjectEditorPane } from "#src/features/workspace/editor/project-editor-pane.tsx";
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
    <main className="empty">
      <h1>{t("Your workspace")}</h1>
      <p>{t("Select a session to continue.")}</p>
    </main>
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
      <div className="session-split">
        <Conversation key={id} c={c} id={id} projects={projects} />
        {activated && !!project?.id.length && (
          <ProjectEditorPane
            key={Array.from(project.id).join("-")}
            c={c}
            projectId={project.id}
          />
        )}
      </div>
    </div>
  );
}
