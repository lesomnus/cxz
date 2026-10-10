import { useRef, useState, type ReactNode } from "react";
import type { Session } from "../gen/cxz/session_pb";
import type { SessionPurgeReply } from "../gen/cxz/session_svc_pb";
import { ActionMenu } from "./action-menu";
import { Button } from "./button";
import { useFloatingCard } from "./floating-card";
import { t, currentLocale } from "./i18n";
import { useLocale } from "./i18n-react";
import { SessionIdentity } from "./session-identity";
import { resumableStates, type SessionOperation } from "./session-actions";
import { formatReset, formatTokens, type SessionInfo } from "./session-info";

export type SessionMenuProps = {
  session?: Session;
  info: SessionInfo;
  busy: boolean;
  manage: (operation: SessionOperation, run: string) => Promise<boolean>;
  previewPurge: () => Promise<SessionPurgeReply | undefined>;
  purge: () => Promise<boolean>;
};

export function SessionMenu({
  session,
  info,
  busy,
  manage,
  previewPurge,
  purge,
}: SessionMenuProps) {
  useLocale();
  const open = useFloatingCard();
  const [copied, setCopied] = useState(false);
  const resume = resumableStates.has(session?.status?.state ?? "");
  const unavailable = busy || !session?.status?.runId;
  const run = session?.status?.runId ?? "";
  async function copyLink() {
    try {
      await navigator.clipboard.writeText(location.href);
      setCopied(true);
    } catch {
      open({ title: t("Copy session link"), content: () => <SessionLink /> });
    }
  }
  function restart() {
    open({
      title: t("Restart session"),
      content: (close) => (
        <SessionConfirmation
          close={close}
          confirm={t("Restart")}
          execute={() => manage("restart", run)}
        >
          <p>
            {t(
              "Active work stops and pending approvals are cleared. Conversation history, login and permission policy are retained.",
            )}
          </p>
          <p className="muted">
            {t(
              "This restarts the agent; it does not recreate the project container.",
            )}
          </p>
        </SessionConfirmation>
      ),
    });
  }
  async function preparePurge() {
    const plan = await previewPurge();
    if (!plan) return;
    open({
      title: t("Purge session"),
      content: (close) => (
        <SessionConfirmation
          close={close}
          confirm={t("Purge permanently")}
          execute={purge}
        >
          <p>
            {t(
              "Permanently deletes this session and its stored data. This cannot be undone.",
            )}
          </p>
          <dl className="session-details purge-plan">
            {plan.targets.map((target, index) => (
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
          {!!plan.retained.length && (
            <p className="muted">
              {t("Retained: {items}", { items: plan.retained.join(", ") })}
            </p>
          )}
        </SessionConfirmation>
      ),
    });
  }
  return (
    <ActionMenu
      label={t("Session menu")}
      placement="above"
      disabled={!session}
      status={copied ? t("Session link copied") : session?.status?.state}
      groups={[
        {
          label: t("Session controls"),
          items: [
            {
              label: resume ? t("Resume session") : t("Stop session"),
              disabled: unavailable,
              run: () => void manage(resume ? "resume" : "stop", run),
            },
            {
              label: t("Restart session"),
              disabled: unavailable,
              run: restart,
            },
            {
              label: t("Purge session"),
              disabled: busy,
              danger: true,
              run: () => void preparePurge(),
            },
          ],
        },
        {
          label: t("Information"),
          items: [
            {
              label: t("Session details"),
              run: () =>
                open({
                  title: t("Session details"),
                  content: () => (
                    <SessionDetails session={session!} info={info} />
                  ),
                }),
            },
            {
              label: t("Copy session link"),
              run: () => void copyLink(),
            },
          ],
        },
      ]}
    />
  );
}

function SessionLink() {
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState("");
  const url = location.href;
  return (
    <div className="session-link">
      <input
        aria-label={t("Session link")}
        readOnly
        value={url}
        onFocus={(event) => event.currentTarget.select()}
      />
      <Button
        disabled={!navigator.clipboard}
        onClick={() =>
          void navigator.clipboard.writeText(url).then(
            () => setCopied(true),
            (error) => setError(String(error)),
          )
        }
      >
        {copied ? t("Copied") : t("Copy")}
      </Button>
      {error && <p role="alert">{error}</p>}
    </div>
  );
}

function SessionConfirmation({
  children,
  close,
  confirm,
  execute,
}: {
  children: ReactNode;
  close: () => void;
  confirm: string;
  execute: () => Promise<boolean>;
}) {
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  async function submit() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setFailed(false);
    try {
      if (await execute()) close();
      else setFailed(true);
    } catch {
      setFailed(true);
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <div className="session-confirmation">
      {children}
      {failed && (
        <p role="alert">
          {t("Action failed. Check session status before retrying.")}
        </p>
      )}
      <div className="session-confirmation-actions">
        <Button type="button" disabled={busy} onClick={close}>
          {t("Cancel")}
        </Button>
        <Button type="button" disabled={busy} onClick={() => void submit()}>
          {busy ? t("Working…") : confirm}
        </Button>
      </div>
    </div>
  );
}

export function SessionDetails({
  session,
  info,
}: {
  session: Session;
  info: SessionInfo;
}) {
  useLocale();
  const date = (value: Session["dateCreated"]) =>
    value
      ? new Date(
          Number(value.seconds) * 1000 + value.nanos / 1e6,
        ).toLocaleString(currentLocale())
      : "—";
  const rows: [string, ReactNode][] = [
    [t("State"), session.status?.state],
    [t("Alias"), session.alias],
    [t("Project"), session.project?.name || session.project?.alias],
    [t("Workspace"), session.project?.workspace],
    [t("Agent"), session.agent],
    [t("Model"), info.model],
    [t("Effort"), info.effort],
    [t("Runtime ID"), session.runtimeId],
    [t("Run ID"), session.status?.runId],
    [t("Provider session ID"), session.status?.vendorId],
    [t("Permission mode"), session.status?.permissionMode],
    [t("Last event"), session.status?.lastSeq?.toString()],
    [t("Pending approvals"), session.status?.pending.length ?? 0],
    [
      t("Remaining usage"),
      info.remaining === undefined ? undefined : `${info.remaining}%`,
    ],
    [t("Usage reset"), formatReset(info.reset)],
    [
      t("Context"),
      `${formatTokens(info.contextUsed)} / ${formatTokens(info.contextWindow)}`,
    ],
    [t("Created"), date(session.dateCreated)],
    [t("Updated"), date(session.dateUpdated)],
  ];
  return (
    <>
      <SessionIdentity session={session} heading />
      <dl className="session-details">
        {rows.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{value || value === 0 ? value : "—"}</dd>
          </div>
        ))}
      </dl>
    </>
  );
}
