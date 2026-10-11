import { useQuery } from "@lesomnus/payday/react";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { TerminalWorkbench } from "./terminal-workbench";
import { TerminalSession } from "./terminal-session";

export function terminalShortcut(
  event: Pick<
    KeyboardEvent,
    | "ctrlKey"
    | "altKey"
    | "metaKey"
    | "shiftKey"
    | "code"
    | "key"
    | "isComposing"
  >,
) {
  return (
    event.ctrlKey &&
    !event.altKey &&
    !event.metaKey &&
    !event.shiftKey &&
    !event.isComposing &&
    (event.code === "Backquote" || event.key === "`")
  );
}

export function WorkspaceTerminal({
  c,
  projectId,
  visible,
  hide,
}: {
  c: Connection;
  projectId: Uint8Array;
  visible: boolean;
  hide: () => void;
}) {
  const project = useQuery(ProjectService.method.get, {
    ref: { key: { case: "id", value: projectId } },
    select: { all: true },
  });
  return (
    <TerminalWorkbench
      visible={visible}
      hide={hide}
      projectError={project.error ? String(project.error) : ""}
      renderSession={(props) => (
        <TerminalSession {...props} c={c} runtimeId={project.data?.runtimeId} />
      )}
    />
  );
}
