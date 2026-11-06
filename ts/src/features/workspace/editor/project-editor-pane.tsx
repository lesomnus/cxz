import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useQuery } from "@lesomnus/payday/react";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { key } from "@lesomnus/payday/store";
import { WorkspaceEditor } from "./workspace-editor";

export function ProjectEditorPane({
  c,
  projectId,
  requestedFile,
  closePreview,
}: {
  c: Connection;
  projectId: Uint8Array;
  requestedFile?: { path: string };
  closePreview?: () => void;
}) {
  useLocale();
  const project = useQuery(ProjectService.method.get, {
    ref: { key: { case: "id", value: projectId } },
    select: { all: true },
  });
  return project.data ? (
    <WorkspaceEditor
      c={c}
      project={project.data}
      requestedFile={requestedFile}
      closePreview={closePreview}
    />
  ) : (
    <aside className="workspace-editor" aria-label={t("Workspace editor")}>
      project.error ? <ErrorNotice error={project.error} /> :
      <p role="status">{t("Loading workspace…")}</p>
    </aside>
  );
}
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
