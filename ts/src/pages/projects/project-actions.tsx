import { useRef, useState } from "react";
import { Button, ConfirmationDialog } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import { ContainerState } from "#gen/cxz/project_pb";
import { containerState } from "./project-state.ts";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { SessionActionIcon } from "#src/features/session/components/session-action-icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { purgeProject } from "./project-purge";
import { stopProject } from "./project-stop";
import styles from "./project-actions.module.css";

type Action = "stop" | "purge";
export function ProjectActions({
  c,
  project,
  sessions,
  purged,
  updated,
}: {
  c: Connection;
  project: Project;
  sessions: Session[];
  purged: () => void;
  updated: (project: Project) => void;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const [target, setTarget] = useState<{ kind: Action; project: Project }>();
  const [error, setError] = useState("");
  const actions = [
    {
      kind: "stop" as const,
      label: t("Stop"),
      description: t("Stop the project container and its sessions."),
      disabled: containerState(project) === ContainerState.STOPPED,
    },
    {
      kind: "purge" as const,
      label: t("Purge"),
      description: t("Permanently delete this project and its owned data."),
      disabled: false,
    },
  ];
  const stopping = target?.kind === "stop";
  return (
    <>
      <div className={styles.rows}>
        {actions.map((action) => (
          <div className={styles.row} key={action.kind}>
            <span className={styles.icon}>
              <SessionActionIcon action={action.kind} />
            </span>
            <p>{action.description}</p>
            <Button
              type="button"
              className={styles.action}
              disabled={action.disabled}
              onClick={(event) => {
                trigger.current = event.currentTarget;
                setError("");
                setTarget({ kind: action.kind, project });
              }}
            >
              {action.label}
            </Button>
          </div>
        ))}
      </div>
      {target && (
        <ConfirmationDialog
          title={stopping ? t("Stop project") : t("Purge project")}
          confirmLabel={stopping ? t("Stop") : t("Purge permanently")}
          close={() => {
            setTarget(undefined);
            trigger.current?.focus({ preventScroll: true });
          }}
          failureMessage={error || t("Action failed. Please try again.")}
          execute={async () => {
            setError("");
            try {
              if (stopping) updated(await stopProject(c, target.project));
              else {
                await purgeProject(c, target.project, sessions);
                purged();
              }
              return true;
            } catch (failure) {
              setError(String(failure));
              return false;
            }
          }}
        >
          <p>
            <strong>
              {target.project.name ||
                target.project.alias ||
                target.project.runtimeId}
            </strong>
          </p>
          <p>
            {stopping
              ? t(
                  "Stops the project container and all its sessions. Conversation history, files and accounts are retained.",
                )
              : t(
                  "Permanently deletes this project, its sessions, conversation history, shared memory and owned resources. This cannot be undone.",
                )}
          </p>
          <p className="muted">
            {stopping
              ? t("The project remains listed and can be started again.")
              : t("Workspace source files and shared accounts are preserved.")}
          </p>
        </ConfirmationDialog>
      )}
    </>
  );
}
