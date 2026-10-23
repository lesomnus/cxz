import { useRef, useState } from "react";
import { Button, ConfirmationDialog } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import { ContainerState } from "#gen/cxz/project_pb";
import { containerState } from "./project-state.ts";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { SessionActionIcon } from "#src/features/session/components/session-action-icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { eraseProject } from "./project-erase";
import { trustRequired } from "./config-trust";
import {
  removeContainer,
  startContainer,
  stopContainer,
} from "./project-container";
import styles from "./project-actions.module.css";

type Action = "stop" | "start" | "remove" | "erase";

// What a button says is the name of the call it makes. Stopping keeps the
// container, starting brings it back, removing throws the container away and
// keeps everything else, and purging is the one that deletes data.
// See docs/plans/project-lifecycle-api.md.
// Each action's glyph. Erasing borrows the trash: it is the one that deletes.
const icons: Record<Action, "stop" | "resume" | "remove" | "purge"> = {
  stop: "stop",
  start: "resume",
  remove: "remove",
  erase: "purge",
};

// `description` is the line beside the button; `body` is what the confirmation
// spells out. They are different lengths on purpose: the row stays scannable
// and the dialog says what survives.
const copy: Record<
  Action,
  {
    label: string;
    description: string;
    title: string;
    confirm: string;
    body: string;
    note: string;
  }
> = {
  stop: {
    label: t("Stop"),
    description: t("Stop the project container and its sessions."),
    title: t("Stop project"),
    confirm: t("Stop"),
    body: t(
      "Stops the project container and all its sessions. The container is kept, so starting it again takes seconds.",
    ),
    note: t("The project remains listed and can be started again."),
  },
  start: {
    label: t("Start"),
    description: t("Start the project container and its runtime."),
    title: t("Start project"),
    confirm: t("Start"),
    body: t(
      "Makes the project run: the container it already has, or a new one built from its configuration, which takes minutes. Agents stay stopped until a session is resumed.",
    ),
    note: t("Workspace source files and shared accounts are preserved."),
  },
  remove: {
    label: t("Remove"),
    description: t("Remove the project container, keeping everything else."),
    title: t("Remove container"),
    confirm: t("Remove"),
    body: t(
      "Removes the project container. The project, its sessions, conversation history and volumes are retained; starting again builds a new container.",
    ),
    note: t("The project remains listed and can be started again."),
  },
  erase: {
    label: t("Erase"),
    description: t("Permanently delete this project and its owned data."),
    title: t("Erase project"),
    confirm: t("Erase permanently"),
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
  erased,
  updated,
}: {
  c: Connection;
  project: Project;
  sessions: Session[];
  erased: () => void;
  updated: (project: Project) => void;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const [target, setTarget] = useState<{ kind: Action; project: Project }>();
  // A start the configuration refused until someone says yes to it: the
  // findings are what they are saying yes to.
  const [trust, setTrust] = useState<{
    project: Project;
    findings: string[];
  }>();
  const [error, setError] = useState("");
  const state = containerState(project);
  const actions: { kind: Action; disabled: boolean }[] = [
    { kind: "stop", disabled: state !== ContainerState.RUNNING },
    {
      kind: "start",
      disabled:
        state === ContainerState.RUNNING ||
        state === ContainerState.UNSPECIFIED,
    },
    {
      kind: "remove",
      disabled:
        state === ContainerState.ABSENT || state === ContainerState.UNSPECIFIED,
    },
    { kind: "erase", disabled: false },
  ];
  return (
    <>
      <div className={styles.rows}>
        {actions.map((action) => (
          <div className={styles.row} key={action.kind}>
            <span className={styles.icon}>
              <SessionActionIcon action={icons[action.kind]} />
            </span>
            <p>{copy[action.kind].description}</p>
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
              if (target.kind === "erase") {
                await eraseProject(c, target.project, sessions);
                erased();
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
              const findings =
                target.kind === "start" ? trustRequired(failure) : undefined;
              if (findings) {
                // Not a failure: a question. This dialog closes and the
                // next one asks it.
                setTrust({ project: target.project, findings });
                return true;
              }
              setError(String(failure));
              return false;
            }
          }}
        >
          <p>
            <strong>{label(target.project)}</strong>
          </p>
          <p>{copy[target.kind].body}</p>
          <p className="muted">{copy[target.kind].note}</p>
        </ConfirmationDialog>
      )}
      {trust && (
        <ConfirmationDialog
          title={t("Trust configuration")}
          confirmLabel={t("Trust and start")}
          close={() => {
            setTrust(undefined);
            trigger.current?.focus({ preventScroll: true });
          }}
          failureMessage={error || t("Action failed. Please try again.")}
          execute={async () => {
            setError("");
            try {
              updated(await startContainer(c, trust.project, true));
              return true;
            } catch (failure) {
              setError(String(failure));
              return false;
            }
          }}
        >
          <p>
            <strong>{label(trust.project)}</strong>
          </p>
          <p>
            {t(
              "This project's devcontainer configuration asks for access beyond its container. Start it only if you trust this configuration.",
            )}
          </p>
          <ul className={styles.findings}>
            {trust.findings.map((finding) => (
              <li key={finding}>
                <code>{finding}</code>
              </li>
            ))}
          </ul>
          <p className="muted">
            {t("You will be asked again if the configuration changes.")}
          </p>
        </ConfirmationDialog>
      )}
    </>
  );
}

function label(project: Project) {
  return project.name || project.alias || project.runtimeId;
}
