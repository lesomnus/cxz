import type { Session } from "#gen/cxz/session_pb";
import { ActionMenu, Button } from "@lesomnus/nox";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { SessionActionIcon } from "#src/features/session/components/session-action-icon.tsx";
import {
  canControl,
  type SessionControlKind,
} from "#src/features/session/model/session-control-batch.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import type { useSessionControls } from "./use-session-controls";
import styles from "./session-table-controls.module.css";

type Controls = ReturnType<typeof useSessionControls>;
export function SessionTableMenu({
  sessions,
  controls,
  bulk = false,
  iconTrigger = false,
  disabled = false,
  addSession,
}: {
  sessions: Session[];
  controls: Controls;
  bulk?: boolean;
  iconTrigger?: boolean;
  disabled?: boolean;
  addSession?: () => void;
}) {
  const operations: SessionControlKind[] = [
    "stop",
    "resume",
    "restart",
    "erase",
  ];
  return (
    <ActionMenu
      label={
        bulk
          ? iconTrigger
            ? t("Selected session actions")
            : t("{count} selected", { count: sessions.length })
          : t("Controls for {name}", {
              name: sessions[0]?.name || sessions[0]?.alias || "",
            })
      }
      triggerContent={
        bulk && !iconTrigger ? (
          <span className={styles.selected}>
            <span className={styles.selectedNumber}>{sessions.length}</span>
            <span>{t("selected")}</span>
          </span>
        ) : undefined
      }
      status={
        bulk ? t("{count} selected", { count: sessions.length }) : undefined
      }
      triggerClassName={
        bulk
          ? iconTrigger
            ? styles.headerTrigger
            : styles.bulkTrigger
          : styles.rowTrigger
      }
      motion={{ from: "top-right", effect: "reveal" }}
      disabled={
        disabled ||
        controls.busy ||
        (!sessions.length && !(bulk && iconTrigger))
      }
      groups={[
        {
          label: t("Controls"),
          items: operations.map((kind) => ({
            label:
              kind === "stop"
                ? t("Stop")
                : kind === "resume"
                  ? t("Resume")
                  : kind === "restart"
                    ? t("Restart")
                    : t("Erase"),
            icon: <SessionActionIcon action={kind} />,
            danger: kind === "erase",
            disabled: !sessions.some((s) => canControl(s, kind)),
            run: () => void controls.request(kind, sessions),
          })),
        },
        ...(addSession
          ? [
              {
                label: "",
                items: [
                  {
                    label: t("Add session"),
                    icon: (
                      <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="1.6"
                        strokeLinecap="round"
                        aria-hidden="true"
                      >
                        <path d="M12 5v14M5 12h14" />
                      </svg>
                    ),
                    run: addSession,
                  },
                ],
              },
            ]
          : []),
      ]}
    />
  );
}

export function SessionTableConfirmation({ controls }: { controls: Controls }) {
  const pending = controls.confirmation;
  if (!pending) return null;
  const single = pending.targets.length === 1;
  return (
    <ConfirmationDialog
      title={
        pending.kind === "erase"
          ? single
            ? t("Erase session")
            : t("Erase sessions")
          : pending.kind === "restart"
            ? single
              ? t("Restart session")
              : t("Restart sessions")
            : single
              ? t("Stop session")
              : t("Stop sessions")
      }
      confirmLabel={
        pending.kind === "erase"
          ? t("Erase")
          : pending.kind === "restart"
            ? t("Restart")
            : t("Stop")
      }
      close={controls.close}
      execute={() => controls.execute(pending.kind, pending.targets)}
      failureMessage={t(
        "Some actions failed. Successful actions will not be repeated. Check session status before retrying.",
      )}
    >
      <p>
        {single
          ? t("This session will be affected.")
          : t("{count} sessions will be affected.", {
              count: pending.targets.length,
            })}
      </p>
      <p>
        {pending.kind === "erase"
          ? single
            ? t(
                "Permanently deletes this session and its stored data. This cannot be undone.",
              )
            : t(
                "Permanently deletes these sessions and their stored data. This cannot be undone.",
              )
          : t(
              "Active work stops and pending approvals are cleared. Conversation history, login and permission policy are retained.",
            )}
      </p>
      {pending.kind === "restart" && (
        <p className="muted">
          {t(
            "This restarts the agent; it does not recreate the project container.",
          )}
        </p>
      )}
      {pending.kind === "stop" && (
        <p className="muted">{t("Sessions already stopped are skipped.")}</p>
      )}
      <ul className={styles.targets}>
        {pending.targets.map((target) => (
          <li key={target.runtimeId}>
            <strong>{target.name}</strong>
          </li>
        ))}
      </ul>
      {!!pending.failures.length && (
        <ul className={styles.failures} role="alert">
          {pending.failures.map((failure) => (
            <li key={failure.target.runtimeId}>
              {failure.target.name}: {failure.error}
            </li>
          ))}
        </ul>
      )}
    </ConfirmationDialog>
  );
}

export function SessionTableFeedback({
  controls,
  inline = false,
}: {
  controls: Controls;
  inline?: boolean;
}) {
  if (!controls.errors.length) return null;
  return (
    <div
      className={`${styles.feedback} ${inline ? styles.inlineFeedback : ""}`}
    >
      <ul className={styles.failures} role="alert">
        {controls.errors.map((failure) => (
          <li key={failure.target.runtimeId}>
            {failure.target.name}: {failure.error}
          </li>
        ))}
      </ul>
      <Button
        type="button"
        aria-label={t("Close")}
        onClick={controls.dismissErrors}
      >
        ×
      </Button>
    </div>
  );
}
