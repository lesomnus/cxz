import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { createPortal } from "react-dom";
import { Button, Card, ScrollArea } from "@lesomnus/nox";
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
  const key = active ? `${active.runId}:${active.requestId}:${active.seq}` : "";
  const id = useId();
  const controls = useRef<HTMLDivElement>(null);
  const stack = useRef<HTMLDivElement>(null);
  const locked = useRef(false);
  const [deciding, setDeciding] = useState(false);
  const [host, setHost] = useState<HTMLElement | null>(null);
  useLayoutEffect(() => {
    setHost(controls.current?.closest<HTMLElement>(".conversation") ?? null);
  }, [key]);
  useLayoutEffect(() => {
    const anchor = controls.current;
    const cards = stack.current;
    if (!host || !anchor || !cards) return;
    const position = () => {
      const bounds = host.getBoundingClientRect();
      const button = anchor.getBoundingClientRect();
      const gap = parseFloat(getComputedStyle(cards).columnGap) || 8;
      cards.style.setProperty(
        "--approval-card-space",
        `${Math.max(0, button.top - bounds.top - gap * 3)}px`,
      );
      const box = cards.getBoundingClientRect();
      const left = Math.max(
        bounds.left + gap,
        Math.min(
          button.left + button.width / 2 - box.width / 2,
          bounds.right - gap - box.width,
        ),
      );
      cards.style.left = `${left - bounds.left - host.clientLeft + host.scrollLeft}px`;
      cards.style.top = `${button.top - bounds.top - host.clientTop + host.scrollTop - gap - box.height}px`;
      cards.dataset.positioned = "true";
    };
    position();
    const observer = new ResizeObserver(position);
    for (const element of [host, anchor, cards, anchor.closest(".composer")])
      if (element) observer.observe(element);
    window.addEventListener("resize", position);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", position);
    };
  }, [host, key, requests.length]);
  if (!active) return null;
  const data = payload(active);
  const unsupported = !!(
    active.text === "mcpServer/elicitation/request" &&
    data.params?.requestedSchema
  );
  const disabled = busy || deciding;
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
        aria-controls={id}
        aria-busy={disabled}
        data-pending-count={requests.length}
      >
        <Button
          type="button"
          className={styles.decision}
          aria-label={t("Deny")}
          title={t("Deny")}
          disabled={disabled}
          onClick={() => decide(false)}
        >
          <svg
            viewBox="0 0 16 16"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path d="m4 4 8 8M12 4l-8 8" />
          </svg>
        </Button>
        <Button
          type="button"
          className={styles.decision}
          aria-label={t("Allow")}
          title={t("Allow")}
          disabled={disabled || unsupported}
          onClick={() => decide(true)}
        >
          <svg
            viewBox="0 0 16 16"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <path d="m3 8 3.2 3.2L13 4.5" />
          </svg>
        </Button>
      </div>
      {host &&
        createPortal(
          <div
            ref={stack}
            id={id}
            className={styles.stack}
            data-approval-stack
            data-pending-count={requests.length}
          >
            {[2, 1]
              .filter((depth) => depth < requests.length)
              .map((depth) => (
                <Card
                  key={depth}
                  title=""
                  hideHeading
                  inert
                  aria-hidden="true"
                  className={styles.back}
                  style={{ "--approval-depth": depth } as CSSProperties}
                >
                  <span />
                </Card>
              ))}
            <Card
              key={key}
              title=""
              hideHeading
              className={styles.request}
              role="region"
              aria-label={approvalTitle(active)}
              aria-live="polite"
              data-request-id={active.requestId}
            >
              <ScrollArea
                className={styles.scroll}
                viewportClassName={styles.viewport}
                scrollbarLabel={t("Request details")}
              >
                <ApprovalContent event={active} />
                {unsupported && (
                  <p className={styles.notice}>
                    {t(
                      "This request form is not supported in the web client yet. Complete it in the TUI.",
                    )}
                  </p>
                )}
              </ScrollArea>
            </Card>
          </div>,
          host,
        )}
    </>
  );
}

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
