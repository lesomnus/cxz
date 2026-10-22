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
import {
  removeContainer,
  startContainer,
  stopContainer,
} from "./project-container";
import styles from "./project-actions.module.css";

type Action = "stop" | "start" | "remove" | "purge";

// What a button says is the name of the call it makes. Stopping keeps the
// container, starting brings it back, removing throws the container away and
// keeps everything else, and purging is the one that deletes data.
// See docs/plans/project-lifecycle-api.md.
const copy: Record<
  Action,
  { label: string; title: string; confirm: string; body: string; note: string }
> = {
  stop: {
    label: t("Stop"),
    title: t("Stop project"),
    confirm: t("Stop"),
    body: t(
      "Stops the project container and all its sessions. The container is kept, so starting it again takes seconds.",
    ),
    note: t("The project remains listed and can be started again."),
  },
  start: {
    label: t("Start"),
    title: t("Start project"),
    confirm: t("Start"),
    body: t(
      "Starts the container this project already has, and its runtime. Agents stay stopped until a session is resumed.",
    ),
    note: t("Workspace source files and shared accounts are preserved."),
  },
  remove: {
    label: t("Remove container"),
    title: t("Remove container"),
    confirm: t("Remove container"),
    body: t(
      "Removes the project container. The project, its sessions, conversation history and volumes are retained; starting again builds a new container.",
    ),
    note: t("The project remains listed and can be started again."),
  },
  purge: {
    label: t("Purge"),
    title: t("Purge project"),
    confirm: t("Purge permanently"),
    body: t(
      "Permanently deletes this project, its sessions, conversation history, shared memory and owned resources. This cannot be undone.",
    ),
    note: t("Workspace source files and shared accounts are preserved."),
  },
};
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
  const state = containerState(project);
  const actions: { kind: Action; disabled: boolean }[] = [
    { kind: "stop", disabled: state !== ContainerState.RUNNING },
    { kind: "start", disabled: state !== ContainerState.STOPPED },
    {
      kind: "remove",
      disabled:
        state === ContainerState.ABSENT ||
        state === ContainerState.UNSPECIFIED,
    },
    { kind: "purge", disabled: false },
  ];
  return (
    <>
      <div className={styles.rows}>
        {actions.map((action) => (
          <div className={styles.row} key={action.kind}>
            <span className={styles.icon}>
              <SessionActionIcon
                action={action.kind === "start" ? "resume" : action.kind}
              />
            </span>
            <p>{copy[action.kind].body}</p>
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
              {copy[action.kind].label}
            </Button>
          </div>
        ))}
      </div>
      {target && (
        <ConfirmationDialog
          title={copy[target.kind].title}
          confirmLabel={copy[target.kind].confirm}
          close={() => {
            setTarget(undefined);
            trigger.current?.focus({ preventScroll: true });
          }}
          failureMessage={error || t("Action failed. Please try again.")}
          execute={async () => {
            setError("");
            try {
              if (target.kind === "purge") {
                await purgeProject(c, target.project, sessions);
                purged();
              } else {
                const control = {
                  stop: stopContainer,
                  start: startContainer,
                  remove: removeContainer,
                }[target.kind];
                updated(await control(c, target.project));
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
          <p>{copy[target.kind].body}</p>
          <p className="muted">{copy[target.kind].note}</p>
        </ConfirmationDialog>
      )}
    </>
  );
}
