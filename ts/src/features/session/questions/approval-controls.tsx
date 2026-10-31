import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { createPortal } from "react-dom";
import { Button, CompactNumber, ScrollArea } from "@lesomnus/nox";
import type { SessionEvent } from "#gen/cxz/session_pb";
import { approvalTitle, payload } from "#src/features/session/model/journal.ts";
import { Markdown } from "#src/shared/content/markdown.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./approval-controls.module.css";
import {
  approvalBacks,
  approvalKey,
  useApprovalPresence,
} from "./use-approval-presence";

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
  const container = useRef<HTMLDivElement>(null);
  const controls = useRef<HTMLDivElement>(null);
  const stack = useRef<HTMLDivElement>(null);
  const front = useRef<HTMLDivElement>(null);
  const closeTimer = useRef(0);
  const locked = useRef(false);
  const [deciding, setDeciding] = useState(false);
  const [host, setHost] = useState<HTMLElement | null>(null);
  const [open, setOpen] = useState(false);
  const { current, visible, leaving, entryDepth, finish, finishFront } =
    useApprovalPresence(requests, open, front);
  const key = approvalKey(current);
  const leavingKey = approvalKey(leaving?.event);
  const backRequests = current
    ? approvalBacks(visible)
    : (leaving?.backs ?? []);
  const show = () => {
    window.clearTimeout(closeTimer.current);
    setOpen(true);
  };
  const closeSoon = () => {
    window.clearTimeout(closeTimer.current);
    closeTimer.current = window.setTimeout(() => {
      if (
        !container.current?.matches(":hover") &&
        !stack.current?.matches(":hover") &&
        !container.current?.querySelector(":focus-visible") &&
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
      const input = anchor
        .closest(".composer")
        ?.querySelector<HTMLElement>(".composer-input");
      if (input) {
        const box = input.getBoundingClientRect();
        cards.style.setProperty(
          "--approval-composer-offset",
          `${Math.max(0, box.top - button.top + gap)}px`,
        );
        cards.style.setProperty("--approval-input-height", `${box.height}px`);
        cards.style.setProperty(
          "--approval-input-radius",
          getComputedStyle(input).borderTopLeftRadius,
        );
      }
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
  }, [host, key, leavingKey, requests.length]);
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
        ref={container}
        className={styles.controls}
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
        <div
          ref={controls}
          className={styles.group}
          role="group"
          aria-label={t("Pending approvals")}
          aria-description={`${t("Pending approvals")}: ${requests.length}`}
          aria-controls={current ? id : undefined}
          aria-busy={busy || deciding}
          data-pending-count={requests.length}
        >
          <Button
            type="button"
            effect="none"
            className={`${styles.decision} ${styles.deny}`}
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
            effect="none"
            className={`${styles.decision} ${styles.allow}`}
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
      </div>
      {(current || leaving) &&
        host &&
        createPortal(
          <div
            ref={stack}
            id={id}
            className={styles.stack}
            data-approval-stack
            data-departing={!!leaving}
            data-closing={!current && !!leaving}
            data-pending-count={requests.length}
            style={!current && leaving ? { height: leaving.height } : undefined}
            onPointerEnter={show}
            onPointerLeave={closeSoon}
            onFocusCapture={focus}
            onBlurCapture={closeSoon}
          >
            {backRequests.map((event, index) => {
              const depth = index + 1;
              return (
                <div
                  key={approvalKey(event)}
                  inert
                  aria-hidden="true"
                  data-approval-back={depth}
                  className={styles.backClip}
                  style={{ "--approval-depth": depth } as CSSProperties}
                >
                  <div
                    className={`${styles.surface} ${styles.back}`}
                    onAnimationEnd={(event) => {
                      if (
                        !current &&
                        depth === backRequests.length &&
                        event.target === event.currentTarget &&
                        event.animationName ===
                          getComputedStyle(event.currentTarget).animationName
                      )
                        finish(leavingKey);
                    }}
                  />
                </div>
              );
            })}
            {current && (
              <div
                ref={front}
                key={key}
                className={`${styles.surface} ${styles.request}`}
                data-motion={entryDepth ? "promote" : "arrive"}
                style={
                  { "--approval-entry-depth": entryDepth } as CSSProperties
                }
                role="region"
                aria-label={approvalTitle(current)}
                aria-live="polite"
                data-request-id={current.requestId}
              >
                <ScrollArea
                  className={styles.scroll}
                  viewportClassName={styles.viewport}
                  scrollbarLabel={t("Request details")}
                >
                  <ApprovalContent event={current} />
                  {unsupported && (
                    <p className={styles.notice}>
                      {t(
                        "This request form is not supported in the web client yet. Complete it in the TUI.",
                      )}
                    </p>
                  )}
                </ScrollArea>
              </div>
            )}
            {leaving && (
              <div
                key={`leaving:${leavingKey}`}
                className={styles.departureClip}
                style={
                  {
                    "--approval-leaving-height": `${leaving.height}px`,
                  } as CSSProperties
                }
                inert
                aria-hidden="true"
              >
                <div
                  className={`${styles.surface} ${styles.request} ${styles.leaving}`}
                  style={{ height: leaving.height }}
                  data-approval-exiting
                  onAnimationEnd={(event) => {
                    if (event.target === event.currentTarget)
                      if (current || !leaving.backs.length) finish(leavingKey);
                      else finishFront(leavingKey);
                  }}
                >
                  <ApprovalContent event={leaving.event} />
                </div>
              </div>
            )}
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
