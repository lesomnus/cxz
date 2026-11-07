import { Icon } from "#src/shared/icons/icon.tsx";
import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { Button, CardStack, CompactNumber } from "@lesomnus/nox";
import type { SessionEvent } from "#gen/cxz/session_pb";
import { approvalTitle, payload } from "#src/features/session/model/journal.ts";
import { Markdown } from "#src/shared/content/markdown.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./approval-controls.module.css";

export function ApprovalControls({
  requests,
  busy,
  reply,
}: {
  requests: readonly SessionEvent[];
  busy: boolean;
  reply: (event: SessionEvent, allow: boolean) => Promise<void>;
}) {
  useLocale();
  const active = requests[0];
  const id = useId();
  const controls = useRef<HTMLDivElement>(null);
  const stack = useRef<HTMLDivElement>(null);
  const closeTimer = useRef(0);
  const locked = useRef(false);
  const [deciding, setDeciding] = useState(false);
  const [host, setHost] = useState<HTMLElement | null>(null);
  const [open, setOpen] = useState(false);
  const key = active ? `${active.runId}:${active.requestId}:${active.seq}` : "";
  const show = () => {
    window.clearTimeout(closeTimer.current);
    setOpen(true);
  };
  const closeSoon = () => {
    window.clearTimeout(closeTimer.current);
    closeTimer.current = window.setTimeout(() => {
      if (
        !controls.current?.matches(":hover") &&
        !stack.current?.matches(":hover") &&
        !controls.current?.querySelector(":focus-visible") &&
        !stack.current?.querySelector(":focus-visible")
      )
        setOpen(false);
    }, 60);
  };
  const focus = () => {
    if (document.activeElement?.matches(":focus-visible")) show();
  };
  useEffect(() => () => window.clearTimeout(closeTimer.current), []);
  useLayoutEffect(() => {
    setHost(controls.current?.closest<HTMLElement>(".conversation") ?? null);
  }, [key]);
  const data = active ? payload(active) : {};
  const unsupported = !!(
    active?.text === "mcpServer/elicitation/request" &&
    data.params?.requestedSchema
  );
  const disabled = !active || busy || deciding;
  async function decide(allow: boolean) {
    if (disabled || locked.current || !active) return;
    locked.current = true;
    setDeciding(true);
    try {
      await reply(active, allow);
    } finally {
      locked.current = false;
      setDeciding(false);
    }
  }
  return (
    <>
      <div
        ref={controls}
        className={styles.group}
        role="group"
        aria-label={t("Pending approvals")}
        aria-description={`${t("Pending approvals")}: ${requests.length}`}
        aria-controls={open && active ? id : undefined}
        aria-busy={busy || deciding}
        data-approval-controls
        data-pending-count={requests.length}
        onPointerEnter={show}
        onPointerLeave={closeSoon}
        onFocusCapture={focus}
        onBlurCapture={closeSoon}
      >
        <CompactNumber
          className={styles.count}
          mode="integer"
          digits={3}
          value={requests.length}
          muted={!requests.length}
          label={`${t("Pending approvals")}: ${requests.length}`}
        />
        <span className={styles.divider} aria-hidden="true" />
        <Button
          type="button"
          effect="none"
          className={`${styles.decision} ${styles.deny}`}
          aria-label={t("Deny")}
          title={t("Deny")}
          disabled={disabled}
          onClick={() => decide(false)}
        >
          <Icon name="deny" width="14" height="14" aria-hidden="true" />
        </Button>
        <Button
          type="button"
          effect="none"
          className={`${styles.decision} ${styles.allow}`}
          aria-label={t("Allow")}
          title={t("Allow")}
          disabled={disabled || unsupported}
          onClick={() => decide(true)}
        >
          <Icon name="allow" width="14" height="14" aria-hidden="true" />
        </Button>
      </div>
      <CardStack
        ref={stack}
        id={id}
        items={requests}
        getKey={requestKey}
        open={open}
        anchor={controls.current}
        container={host}
        occlusionBoundary={controls.current?.closest<HTMLElement>(
          ".composer-wrapper",
        )}
        exitTarget={controls.current
          ?.closest(".composer")
          ?.querySelector<HTMLElement>(".composer-input")}
        scrollbarLabel={t("Request details")}
        getItemProps={(event, state) =>
          ({
            role: "region",
            "aria-label": approvalTitle(event),
            "aria-live": state.active ? "polite" : undefined,
            "data-request-id": event.requestId,
          }) as React.HTMLAttributes<HTMLDivElement>
        }
        renderItem={(event) => (
          <>
            <ApprovalContent event={event} />
            {unsupportedRequest(event) && (
              <p className={styles.notice}>
                {t(
                  "This request form is not supported in the web client yet. Complete it in the TUI.",
                )}
              </p>
            )}
          </>
        )}
        onPointerEnter={show}
        onPointerLeave={closeSoon}
        onFocusCapture={focus}
        onBlurCapture={closeSoon}
      />
    </>
  );
}

const requestKey = (event: SessionEvent) =>
  `${event.runId}:${event.requestId}:${event.seq}`;
const unsupportedRequest = (event: SessionEvent) =>
  !!(
    event.text === "mcpServer/elicitation/request" &&
    payload(event).params?.requestedSchema
  );

function ApprovalContent({ event }: { event: SessionEvent }) {
  const root = payload(event);
  const data = root.params ?? root;
  const metadata = data._meta ?? {};
  const text = (value: unknown) => (typeof value === "string" ? value : "");
  const command = text(data.command) || text(data.input?.command);
  const message =
    text(data.message) ||
    text(data.reason) ||
    text(data.description) ||
    text(data.input?.description);
  const cwd = text(data.cwd) || text(data.grantRoot);
  const url = text(data.url);
  const description = text(metadata.tool_description);
  const input =
    metadata.tool_params ??
    data.permissions ??
    data.changes ??
    (!command ? data.input : undefined);
  return (
    <div className={styles.content}>
      {message && <Markdown text={message} />}
      {description && description !== message && (
        <p className={styles.notice}>{description}</p>
      )}
      {command && (
        <pre>
          <code>{command}</code>
        </pre>
      )}
      {cwd && <small className={styles.path}>{cwd}</small>}
      {url &&
        (/^https?:\/\//i.test(url) ? (
          <a href={url} target="_blank" rel="noopener noreferrer">
            {url}
          </a>
        ) : (
          <code>{url}</code>
        ))}
      {input && (
        <pre>
          <code>{JSON.stringify(input, null, 2)}</code>
        </pre>
      )}
      {!message && !command && !input && !url && (
        <pre>
          <code>{JSON.stringify(data, null, 2)}</code>
        </pre>
      )}
    </div>
  );
}
