import type { Memory } from "#gen/cxz/memory_pb";
import {
  emptyWorkMemories,
  type WorkMemoryState,
  type WorkMemorySource,
} from "#src/features/session/model/work-memories.ts";
import { CreatedMemoriesMenu } from "./created-memories-menu";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { eventListSource } from "#src/features/session/model/event-source.ts";
import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  Button,
  CompactNumber,
  ElapsedTime,
  Drawer,
  Card,
  type ButtonProps,
} from "@lesomnus/nox";
import type { SessionEvent } from "#gen/cxz/session_pb";
import type { WorkGroup } from "#src/features/session/model/work-groups.ts";
import type { Task } from "#gen/cxz/task_pb";
import type {
  WorkTaskSource,
  WorkTaskState,
} from "#src/features/session/model/work-tasks.ts";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { CategoryIcon } from "#src/shared/components/category-icon.tsx";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { VirtualMessages } from "./virtual-messages";
import { ActivityWorkContext } from "./activity-card";
import styles from "./work-dialog.module.css";
import type { WorkHistory } from "./transcript";
import { CreatedTasksMenu } from "./created-tasks-menu";

function ServiceCount({
  service,
  count,
  partial,
  ...props
}: {
  service: "cxz_memory" | "cxz_tasks";
  count: number;
  partial: boolean;
} & Pick<
  ButtonProps,
  | "onClick"
  | "aria-expanded"
  | "aria-controls"
  | "aria-haspopup"
  | "aria-busy"
  | "title"
>) {
  const label = t(
    service === "cxz_memory" ? "Created memories" : "Created tasks",
  );
  return (
    <Button
      {...props}
      type="button"
      className={styles.service}
      aria-label={`${label}: ${count}${partial ? "+" : ""}`}
      data-work-service={service}
      data-empty={count === 0 || undefined}
      disabled={count === 0}
    >
      <span aria-hidden="true">
        <ResourceIcon kind={service === "cxz_memory" ? "memory" : "tasks"} />
      </span>
      <span className={styles.count} aria-hidden="true">
        <CompactNumber value={count} mode="integer" digits={3} />
        <span className={styles.countSuffix}>{partial ? "+" : ""}</span>
      </span>
    </Button>
  );
}

export function WorkSummary({
  group,
  taskSource,
  tasks,
  taskState,
  memorySource,
  memories = [],
  memoryState = emptyWorkMemories,
  expanded,
  open,
  partial = false,
}: {
  group: WorkGroup;
  taskSource?: WorkTaskSource;
  tasks: Task[];
  taskState: WorkTaskState;
  memorySource?: WorkMemorySource;
  memories?: Memory[];
  memoryState?: WorkMemoryState;
  expanded: boolean;
  open: (trigger: HTMLButtonElement) => void;
  partial?: boolean;
}) {
  useLocale();
  const { id, durationMs } = group;
  const count = group.events.length;
  const menuId = useId();
  const [menu, setMenu] = useState<{
    kind: "tasks" | "memory";
    anchor: HTMLButtonElement;
  } | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const closeMenu = useCallback((restoreFocus = false) => {
    setMenu(null);
    if (restoreFocus) triggerRef.current?.focus({ preventScroll: true });
  }, []);
  useEffect(() => {
    if (menu && !(menu.kind === "tasks" ? tasks.length : memories.length))
      closeMenu();
  }, [menu, tasks.length, memories.length, closeMenu]);
  const toggle = (kind: "tasks" | "memory", anchor: HTMLButtonElement) => {
    triggerRef.current = anchor;
    setMenu((current) => (current?.kind === kind ? null : { kind, anchor }));
  };
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
      <div className={styles.counters}>
        <Button
          type="button"
          aria-label={t("View details")}
          aria-description={countLabel}
          aria-expanded={expanded}
          aria-controls={expanded ? "work-dialog" : undefined}
          data-empty={count === 0 || undefined}
          disabled={count === 0}
          onClick={(event) => open(event.currentTarget)}
        >
          <CategoryIcon />
          <span className={styles.count} aria-hidden="true">
            <CompactNumber value={count} mode="integer" digits={3} />
            <span className={styles.countSuffix}>{partial ? "+" : ""}</span>
          </span>
        </Button>
        <ServiceCount
          service="cxz_memory"
          count={memories.length}
          partial={partial}
          aria-busy={memoryState.loading || undefined}
          title={memoryState.error || undefined}
          aria-haspopup="menu"
          aria-expanded={menu?.kind === "memory"}
          aria-controls={menu?.kind === "memory" ? menuId : undefined}
          onClick={(event) => toggle("memory", event.currentTarget)}
        />
        <ServiceCount
          service="cxz_tasks"
          count={tasks.length}
          partial={partial}
          aria-busy={taskState.loading || undefined}
          title={taskState.error || undefined}
          aria-haspopup="menu"
          aria-expanded={menu?.kind === "tasks"}
          aria-controls={menu?.kind === "tasks" ? menuId : undefined}
          onClick={(event) => toggle("tasks", event.currentTarget)}
        />
      </div>
      {durationMs !== undefined && (
        <ElapsedTime
          durationMs={durationMs}
          label={t("Response elapsed time")}
        />
      )}
      {menu?.kind === "memory" && memories.length > 0 && memorySource && (
        <CreatedMemoriesMenu
          id={menuId}
          anchor={menu.anchor}
          source={memorySource}
          memories={memories}
          state={memoryState}
          partial={partial}
          close={closeMenu}
        />
      )}
      {menu?.kind === "tasks" && tasks.length > 0 && taskSource && (
        <CreatedTasksMenu
          id={menuId}
          anchor={menu.anchor}
          project={taskSource.project}
          tasks={tasks}
          state={taskState}
          partial={partial}
          close={closeMenu}
        />
      )}
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
      <Card
        fill
        bodyPadding="var(--ui-space)"
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
              {historyError && <ErrorNotice as="span" error={historyError} />}
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
      </Card>
    </Drawer>
  );
}
