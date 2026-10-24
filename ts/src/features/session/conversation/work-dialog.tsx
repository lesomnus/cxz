import { eventListSource } from "#src/features/session/model/event-source.ts";
import { useId, useRef, useState, type ReactNode } from "react";
import { Button, ElapsedTime, Drawer, DrawerCard } from "@lesomnus/nox";
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
  id,
  count,
  durationMs,
  expanded,
  open,
  partial = false,
}: {
  id: string;
  count: number;
  durationMs?: number;
  expanded: boolean;
  open: (trigger: HTMLButtonElement) => void;
  partial?: boolean;
}) {
  useLocale();
  const tooltip = useId();
  const countLabel = t(
    partial
      ? "{count}+ tasks performed"
      : count === 1
        ? "{count} task performed"
        : "{count} tasks performed",
    { count },
  );
  return (
    <div className={styles.summary} data-work-summary={id}>
      {durationMs !== undefined && (
        <ElapsedTime
          durationMs={durationMs}
          label={t("Response elapsed time")}
        />
      )}
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
            <span className={styles.countValue}>{count}</span>
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
  source,
  render,
  close,
  closing = false,
  history,
  container,
  onClosed,
}: {
  source: () => WorkGroup;
  render: (event: SessionEvent) => ReactNode;
  close: () => void;
  closing?: boolean;
  history?: WorkHistory;
  container: HTMLElement;
  onClosed: () => void;
}) {
  const work = source();
  useLocale();
  const pane = useRef<HTMLDivElement>(null);
  const follow = useRef(false);
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
  return (
    <Drawer
      id="work-dialog"
      open={!closing}
      onClose={close}
      onClosed={onClosed}
      container={container}
      restoreFocus={false}
      aria-label={t("Tasks between responses")}
    >
      <DrawerCard
        title={`${t("Tasks")} · ${work.events.length}`}
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
                source={eventListSource(work.events)}
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
      </DrawerCard>
    </Drawer>
  );
}
