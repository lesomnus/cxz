import type { Session } from "#gen/cxz/session_pb";
import { ActionMenu, Button, ConfirmationDialog } from "@lesomnus/nox";
import { SessionActionIcon } from "#src/features/session/components/session-action-icon.tsx";
import {
  canControl,
  type SessionControlKind,
} from "#src/features/session/model/session-control-batch.ts";
import { resumableStates } from "#src/features/session/model/session-actions.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import type { useSessionControls } from "./use-session-controls";
import styles from "./project-session-controls.module.css";

type Controls = ReturnType<typeof useSessionControls>;
export function ProjectSessionMenu({
  sessions,
  controls,
  bulk = false,
  disabled = false,
}: {
  sessions: Session[];
  controls: Controls;
  bulk?: boolean;
  disabled?: boolean;
}) {
  const operations: SessionControlKind[] = bulk
    ? ["stop", "resume", "restart", "purge"]
    : [
        resumableStates.has(sessions[0]?.status?.state ?? "")
          ? "resume"
          : "stop",
        "restart",
        "purge",
      ];
  return (
    <ActionMenu
      label={
        bulk
          ? t("Selected session actions")
          : t("Controls for {name}", {
              name: sessions[0]?.name || sessions[0]?.alias || "",
            })
      }
      triggerContent={bulk ? t("Actions") : undefined}
      triggerClassName={bulk ? styles.bulkTrigger : styles.rowTrigger}
      motion={{ from: "top-right", effect: "reveal" }}
      disabled={disabled || controls.busy || !sessions.length}
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
                    : t("Purge"),
            icon: <SessionActionIcon action={kind} />,
            danger: kind === "purge",
            disabled: !sessions.some((s) => canControl(s, kind)),
            run: () => void controls.request(kind, sessions),
          })),
        },
      ]}
    />
  );
}

export function ProjectSessionConfirmation({
  controls,
}: {
  controls: Controls;
}) {
  const pending = controls.confirmation;
  if (!pending) return null;
  const single = pending.targets.length === 1;
  return (
    <ConfirmationDialog
      title={
        pending.kind === "purge"
          ? single
            ? t("Purge session")
            : t("Purge sessions")
          : pending.kind === "restart"
            ? single
              ? t("Restart session")
              : t("Restart sessions")
            : single
              ? t("Stop session")
              : t("Stop sessions")
      }
      confirmLabel={
        pending.kind === "purge"
          ? t("Purge permanently")
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
        {pending.kind === "purge"
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
            {pending.plans.get(target.runtimeId)?.targets.map((item, i) => (
              <div className={styles.purgeTarget} key={i}>
                <span>{item.kind}</span>
                <code>{item.path}</code>
                <small>
                  {t("{files} files · {bytes} bytes", {
                    files: item.files,
                    bytes: item.bytes.toString(),
                  })}
                </small>
              </div>
            ))}
            {!!pending.plans.get(target.runtimeId)?.retained.length && (
              <p className="muted">
                {t("Retained: {items}", {
                  items: pending.plans
                    .get(target.runtimeId)!
                    .retained.join(", "),
                })}
              </p>
            )}
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

export function ProjectSessionFeedback({ controls }: { controls: Controls }) {
  if (!controls.errors.length) return null;
  return (
    <div className={styles.feedback}>
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
