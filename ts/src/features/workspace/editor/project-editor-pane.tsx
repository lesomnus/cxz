import { t } from "../../../shared/i18n/i18n";
import { useLocale } from "../../../shared/i18n/i18n-react";
import { useQuery } from "@lesomnus/payday/react";
import { ProjectService } from "../../../../gen/cxz/project_svc_pb";
import { Connection, ref } from "../../../shared/api/connection";
import { key } from "@lesomnus/payday/store";
import { WorkspaceEditor } from "./workspace-editor";

export function ProjectEditorPane({
  c,
  projectId,
}: {
  c: Connection;
  projectId: Uint8Array;
}) {
  useLocale();
  const project = useQuery(ProjectService.method.get, {
    ref: { key: { case: "id", value: projectId } },
    select: { all: true },
  });
  return project.data ? (
    <WorkspaceEditor c={c} project={project.data} />
  ) : (
    <aside className="workspace-editor" aria-label={t("Workspace editor")}>
      <p role={project.error ? "alert" : "status"}>
        {project.error ? String(project.error) : t("Loading workspace…")}
      </p>
    </aside>
  );
}
