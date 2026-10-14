import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  Button,
  DialogCard,
  DialogSurface,
  dialogClassName,
} from "@lesomnus/cxz-ui";
import { createPortal } from "react-dom";
import type { SessionEvent } from "#gen/cxz/session_pb";
import type { WorkGroup } from "#src/features/session/model/work-groups.ts";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { CategoryIcon } from "#src/shared/components/category-icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { VirtualMessages } from "./virtual-messages";
import { ActivityWorkContext } from "./activity-card";
import styles from "./work-dialog.module.css";
import type { WorkHistory } from "./transcript";

export function WorkSummary({
  work,
  expanded,
  open,
  partial = false,
}: {
  work: WorkGroup;
  expanded: boolean;
  open: (trigger: HTMLButtonElement) => void;
  partial?: boolean;
}) {
  useLocale();
  const tooltip = useId();
  const countLabel = t(
    partial
      ? "{count}+ tasks performed"
      : work.events.length === 1
        ? "{count} task performed"
        : "{count} tasks performed",
    { count: work.events.length },
  );
  return (
    <div className={styles.summary} data-work-summary={work.id}>
      <span className="meta-popover">
        <Button
          type="button"
          aria-label={t("View details")}
          aria-describedby={tooltip}
          aria-expanded={expanded}
          aria-controls={expanded ? "work-dialog" : undefined}
          onClick={(event) => open(event.currentTarget)}
        >
          <span className={styles.count} aria-hidden="true">
            <span className={styles.countValue}>{work.events.length}</span>
            <span className={styles.countSuffix}>{partial ? "+" : ""}</span>
          </span>
          <CategoryIcon />
        </Button>
        <span id={tooltip} className="meta-tooltip" role="tooltip">
          {countLabel} · {t("View details")}
        </span>
      </span>
    </div>
  );
}

export function WorkDialog({
  work,
  render,
  close,
  closing = false,
  history,
  container,
  onClosed,
}: {
  work: WorkGroup;
  render: (event: SessionEvent) => ReactNode;
  close: () => void;
  closing?: boolean;
  history?: WorkHistory;
  container: HTMLElement;
  onClosed: () => void;
}) {
  useLocale();
  const pane = useRef<HTMLDivElement>(null);
  const follow = useRef(false);
  const dialog = useRef<HTMLDialogElement>(null);
  const latestClosed = useRef(onClosed);
  latestClosed.current = onClosed;
  const [selectedTask, selectTask] = useState<SessionEvent>();
  const selectedEvent =
    selectedTask &&
    work.events.find(
      (event) =>
        event.seq === selectedTask.seq ||
        (selectedTask.requestId &&
          event.requestId === selectedTask.requestId &&
          event.runId === selectedTask.runId),
    );
  const selected = selectedEvent?.seq.toString();
  const controls = useId();
  const select = (seq: string | undefined) =>
    selectTask(work.events.find((event) => event.seq.toString() === seq));
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [historyError, setHistoryError] = useState("");
  useLayoutEffect(() => {
    const node = dialog.current!;
    const position = () => {
      const bounds = container.getBoundingClientRect();
      Object.assign(node.style, {
        left: `${bounds.left}px`,
        top: `${bounds.top}px`,
        width: `${bounds.width}px`,
        height: `${bounds.height}px`,
      });
    };
    node.showModal();
    position();
    node
      .querySelector<HTMLButtonElement>("button:not(:disabled)")
      ?.focus({ preventScroll: true });
    const observer = new ResizeObserver(position);
    observer.observe(container);
    window.addEventListener("resize", position);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", position);
      node.close();
      latestClosed.current();
    };
  }, [container]);
  return createPortal(
    <dialog
      ref={dialog}
      id="work-dialog"
      className={`${dialogClassName} ${styles.overlay}`}
      aria-label={t("Tasks between responses")}
      data-closing={closing}
      aria-hidden={closing}
      inert={closing}
      onKeyDown={(event) => {
        event.stopPropagation();
        if (event.key !== "Escape" || event.defaultPrevented) return;
        event.preventDefault();
        event.stopPropagation();
        close();
      }}
      onCancel={(event) => {
        event.preventDefault();
        if (!closing) close();
      }}
      onClick={(event) => {
        if (event.target !== event.currentTarget) return;
        const bounds = event.currentTarget.getBoundingClientRect();
        if (
          event.clientX < bounds.left ||
          event.clientX > bounds.right ||
          event.clientY < bounds.top ||
          event.clientY > bounds.bottom
        )
          close();
      }}
    >
      <div
        className={styles.backdrop}
        data-work-backdrop="true"
        aria-hidden="true"
        onClick={() => close()}
      />
      <div className={styles.frame}>
        <DialogSurface motion="from-right" closing={closing}>
          <DialogCard
            title={`${t("Tasks")} · ${work.events.length}`}
            className={styles.card}
            close={() => close()}
            closeLabel={t("Close task dialog")}
            footer={
              !work.hasBoundary &&
              history?.hasOlder && (
                <Button
                  type="button"
                  disabled={loadingOlder}
                  onClick={async () => {
                    setLoadingOlder(true);
                    setHistoryError("");
                    try {
                      await history.loadOlder();
                    } catch (error) {
                      setHistoryError(String(error));
                    } finally {
                      setLoadingOlder(false);
                    }
                  }}
                >
                  {t(loadingOlder ? "Loading…" : "Load earlier tasks")}
                  {historyError && <span role="alert">{historyError}</span>}
                </Button>
              )
            }
          >
            <div className={styles.layout}>
              <ActivityWorkContext value={{ selected, select, controls }}>
                <PanelScroll viewportRef={pane}>
                  <VirtualMessages
                    pane={pane}
                    events={work.events}
                    follow={follow}
                    render={render}
                    changed={() => {}}
                    precedingPrompt={undefined}
                    onPromptChange={() => {}}
                    foldActivities={false}
                  />
                </PanelScroll>
              </ActivityWorkContext>
              {selectedEvent && (
                <div
                  key={`${selectedTask!.runId}/${selectedTask!.requestId || selectedTask!.seq}`}
                  id={controls}
                  className={styles.details}
                  role="region"
                  aria-label={t("Task details")}
                >
                  <ActivityWorkContext
                    value={{ selected, select, controls, detailsOnly: true }}
                  >
                    {render(selectedEvent)}
                  </ActivityWorkContext>
                </div>
              )}
            </div>
          </DialogCard>
        </DialogSurface>
      </div>
    </dialog>,
    document.body,
  );
}
