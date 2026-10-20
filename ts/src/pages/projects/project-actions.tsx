import { useRef, useState } from "react";
import { Button, ConfirmationDialog } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { SessionActionIcon } from "#src/features/session/components/session-action-icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { purgeProject } from "./project-purge";
import styles from "./project-actions.module.css";

export function ProjectActions({
  c,
  project,
  sessions,
  purged,
}: {
  c: Connection;
  project: Project;
  sessions: Session[];
  purged: () => void;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const [target, setTarget] = useState<Project>();
  const [error, setError] = useState("");
  return (
    <>
      <div className={styles.row}>
        <p>{t("Permanently delete this project and its owned data.")}</p>
        <Button
          ref={trigger}
          type="button"
          className={styles.action}
          onClick={() => {
            setError("");
            setTarget(project);
          }}
        >
          <span className={styles.icon}>
            <SessionActionIcon action="purge" />
          </span>
          <span className={styles.label}>{t("Purge")}</span>
        </Button>
      </div>
      {target && (
        <ConfirmationDialog
          title={t("Purge project")}
          confirmLabel={t("Purge permanently")}
          close={() => {
            setTarget(undefined);
            trigger.current?.focus({ preventScroll: true });
          }}
          failureMessage={error || t("Action failed. Please try again.")}
          execute={async () => {
            setError("");
            try {
              await purgeProject(c, target, sessions);
              purged();
              return true;
            } catch (failure) {
              setError(String(failure));
              return false;
            }
          }}
        >
          <p>
            <strong>{target.name || target.alias || target.runtimeId}</strong>
          </p>
          <p>
            {t(
              "Permanently deletes this project, its sessions, conversation history, shared memory and owned resources. This cannot be undone.",
            )}
          </p>
          <p className="muted">
            {t("Workspace source files and shared accounts are preserved.")}
          </p>
        </ConfirmationDialog>
      )}
    </>
  );
}
