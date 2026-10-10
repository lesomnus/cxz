import { useState } from "react";
import type { Session } from "../gen/cxz/session_pb";
import type { SessionPurgeReply } from "../gen/cxz/session_svc_pb";
import { ActionMenu } from "./action-menu";
import { ConfirmationDialog } from "./confirmation-dialog";
import { useFloatingCard } from "./floating-card";
import { t } from "./i18n";
import { useLocale } from "./i18n-react";
import { SessionDetails } from "./session-details";
import type { SessionField } from "./session-edit";
import { resumableStates, type SessionOperation } from "./session-actions";
import type { SessionInfo } from "./session-info";

export type SessionMenuProps = {
  session?: Session;
  info: SessionInfo;
  busy: boolean;
  manage: (operation: SessionOperation, run: string) => Promise<boolean>;
  previewPurge: () => Promise<SessionPurgeReply | undefined>;
  purge: () => Promise<boolean>;
  edit: (field: SessionField, value: string) => Promise<Session>;
};

export function SessionMenu({
  session,
  info,
  busy,
  manage,
  previewPurge,
  purge,
  edit,
}: SessionMenuProps) {
  useLocale();
  const open = useFloatingCard();
  const resume = resumableStates.has(session?.status?.state ?? "");
  const unavailable = busy || !session?.status?.runId;
  const run = session?.status?.runId ?? "";
  const [confirmation, setConfirmation] = useState<{
    sessionId: string;
    run: string;
    kind: "stop" | "restart" | "purge";
    plan?: SessionPurgeReply;
  }>();
  function confirm(kind: "stop" | "restart") {
    if (session) setConfirmation({ sessionId: session.runtimeId, run, kind });
  }
  async function preparePurge() {
    const plan = await previewPurge();
    if (plan && session)
      setConfirmation({
        sessionId: session.runtimeId,
        run,
        kind: "purge",
        plan,
      });
  }
  return (
    <>
      <ActionMenu
        label={t("Session menu")}
        placement="above"
        disabled={!session}
        status={session?.status?.state}
        groups={[
          {
            label: t("Controls"),
            items: [
              {
                label: resume ? t("Resume") : t("Stop"),
                icon: <SessionActionIcon action={resume ? "resume" : "stop"} />,
                disabled: unavailable,
                run: () =>
                  resume ? void manage("resume", run) : confirm("stop"),
              },
              {
                label: t("Restart"),
                icon: <SessionActionIcon action="restart" />,
                disabled: unavailable,
                run: () => confirm("restart"),
              },
              {
                label: t("Purge"),
                icon: <SessionActionIcon action="purge" />,
                disabled: busy,
                danger: true,
                run: () => void preparePurge(),
              },
            ],
          },
          {
            label: "",
            items: [
              {
                label: t("Details"),
                icon: <SessionActionIcon action="details" />,
                run: () =>
                  open({
                    title: t("Session details"),
                    content: () => (
                      <SessionDetails
                        session={session!}
                        info={info}
                        edit={edit}
                      />
                    ),
                  }),
              },
            ],
          },
        ]}
      />
      {confirmation?.sessionId === session?.runtimeId && confirmation && (
        <ConfirmationDialog
          key={`${confirmation.sessionId}:${confirmation.kind}:${confirmation.run}`}
          title={
            confirmation.kind === "purge"
              ? t("Purge session")
              : confirmation.kind === "restart"
                ? t("Restart session")
                : t("Stop session")
          }
          confirmLabel={
            confirmation.kind === "purge"
              ? t("Purge permanently")
              : confirmation.kind === "restart"
                ? t("Restart")
                : t("Stop")
          }
          close={() => setConfirmation(undefined)}
          failureMessage={t(
            "Action failed. Check session status before retrying.",
          )}
          execute={() =>
            confirmation.kind === "purge"
              ? purge()
              : manage(confirmation.kind, confirmation.run)
          }
        >
          {confirmation.kind === "purge" ? (
            <>
              <p>
                {t(
                  "Permanently deletes this session and its stored data. This cannot be undone.",
                )}
              </p>
              <dl className="session-details purge-plan">
                {confirmation.plan!.targets.map((target, index) => (
                  <div key={index}>
                    <dt>{target.kind}</dt>
                    <dd>
                      <code>{target.path}</code>
                      <small>
                        {t("{files} files · {bytes} bytes", {
                          files: target.files,
                          bytes: target.bytes.toString(),
                        })}
                      </small>
                    </dd>
                  </div>
                ))}
              </dl>
              {!!confirmation.plan!.retained.length && (
                <p className="muted">
                  {t("Retained: {items}", {
                    items: confirmation.plan!.retained.join(", "),
                  })}
                </p>
              )}
            </>
          ) : (
            <>
              <p>
                {t(
                  "Active work stops and pending approvals are cleared. Conversation history, login and permission policy are retained.",
                )}
              </p>
              {confirmation.kind === "restart" && (
                <p className="muted">
                  {t(
                    "This restarts the agent; it does not recreate the project container.",
                  )}
                </p>
              )}
            </>
          )}
        </ConfirmationDialog>
      )}
    </>
  );
}

function SessionActionIcon({
  action,
}: {
  action: "stop" | "resume" | "restart" | "purge" | "details";
}) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {action === "stop" && <rect x="4" y="4" width="8" height="8" rx="1" />}
      {action === "resume" && <path d="m5 3 7 5-7 5Z" />}
      {action === "restart" && (
        <>
          <path d="M13 7a5 5 0 1 0-.8 3.7M13 3v4H9" />
        </>
      )}
      {action === "purge" && (
        <>
          <path d="M3 4h10M6 4V2h4v2M4 4l.7 9h6.6l.7-9M6.5 6.5v4M9.5 6.5v4" />
        </>
      )}
      {action === "details" && (
        <>
          <circle cx="8" cy="8" r="5.5" />
          <path d="M8 7.5v3M8 5.2v.1" />
        </>
      )}
    </svg>
  );
}
