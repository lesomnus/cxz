import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  Fragment,
  type ReactNode,
  type DragEvent,
  type Ref,
} from "react";
import { Button } from "../components/button/button";
import {
  SplitLayout,
  setSplitRatio,
} from "../components/split-layout/split-layout";
import { useUI } from "../providers/ui-provider";
import {
  addTerminal,
  closeTerminal,
  initialTerminalLayout,
  moveTerminal,
  paneOrder,
  selectTerminal,
  type DockEdge,
  type TerminalGroup,
} from "./terminal-layout";
import styles from "./terminal-workbench.module.css";

export type TerminalSessionStatus = {
  state: string;
  ended: boolean;
  clipboardNotice: "copied" | "failed" | "";
};
export type TerminalSessionHandle = {
  focus: () => void;
  reconnect: () => void;
};

export type TerminalSurfaceProps = {
  id: string;
  groupId: string;
  active: boolean;
  focused: boolean;
  visible: boolean;
  placement: unknown;
  getHost: (group: string) => HTMLElement | null;
  onStatus: (id: string, status: TerminalSessionStatus) => void;
  onFocus: (id: string) => void;
  ref: Ref<TerminalSessionHandle>;
};

export type TerminalWorkbenchProps = {
  visible: boolean;
  hide: () => void;
  renderSession: (props: TerminalSurfaceProps) => ReactNode;
  error?: string;
  className?: string;
  hideShortcut?: string;
  formatStatus?: (state: string) => string;
};

const dragType = "application/x-terminal-workbench";
export function TerminalWorkbench({
  visible,
  hide,
  renderSession,
  error = "",
  className = "",
  hideShortcut,
  formatStatus = (state) => state,
}: TerminalWorkbenchProps) {
  const { t } = useUI();
  const prefix = useId();
  const [layout, setLayout] = useState(initialTerminalLayout);
  const [focused, setFocused] = useState("terminal-1");
  const [statuses, setStatuses] = useState<
    Record<string, TerminalSessionStatus>
  >({});
  const [height, setHeight] = useState<number>();
  const [focusRequest, requestFocus] = useState(0);
  const nextId = useRef(2);
  const nextDock = useRef(1);
  const panel = useRef<HTMLElement>(null);
  const slots = useRef(new Map<string, HTMLElement>());
  const handles = useRef(new Map<string, TerminalSessionHandle>());
  const dragging = useRef<string | null>(null);
  const resizeOrigin = useRef({ y: 0, height: 0 });
  const [drop, setDrop] = useState<{
    group: string;
    edge: DockEdge;
    index?: number;
  } | null>(null);
  const getHost = useCallback(
    (id: string) => slots.current.get(id) ?? null,
    [],
  );
  const onStatus = useCallback(
    (id: string, value: TerminalSessionStatus) =>
      setStatuses((current) => {
        const old = current[id];
        return old?.state === value.state &&
          old.ended === value.ended &&
          old.clipboardNotice === value.clipboardNotice
          ? current
          : { ...current, [id]: value };
      }),
    [],
  );
  const order = paneOrder(layout.tree);
  const sessions = layout.groups
    .flatMap((group) =>
      group.tabs.map((id) => ({
        id,
        groupId: group.id,
        active: group.active === id,
      })),
    )
    .sort((a, b) => Number(a.id.slice(9)) - Number(b.id.slice(9)));
  const name = (id: string) => t("Terminal {number}", { number: id.slice(9) });
  function add(group = order[0] ?? "", edge: DockEdge = "center") {
    const id = `terminal-${nextId.current++}`;
    setLayout((current) => addTerminal(current, id, group, edge));
    setFocused(id);
    requestFocus((value) => value + 1);
  }
  useEffect(() => {
    if (visible && !layout.tree) add();
  }, [visible, layout.tree]);
  useLayoutEffect(() => {
    if (visible) handles.current.get(focused)?.focus();
  }, [focusRequest, visible]);
  function activate(id: string) {
    setLayout((current) => selectTerminal(current, id));
    setFocused(id);
    handles.current.get(id)?.focus();
  }
  function close(id: string) {
    const next = closeTerminal(layout, id);
    setLayout(next);
    setStatuses((current) => {
      const result = { ...current };
      delete result[id];
      return result;
    });
    if (focused === id)
      setFocused(
        next.groups.find(
          (group) =>
            group.id ===
            layout.groups.find((group) => group.tabs.includes(id))?.id,
        )?.active ??
          next.groups[0]?.active ??
          "",
      );
    if (!next.tree) hide();
    requestFocus((value) => value + 1);
  }
  function maxHeight() {
    return Math.max(
      120,
      (panel.current?.parentElement?.clientHeight ?? window.innerHeight) * 0.7,
    );
  }
  function resize(value: number) {
    setHeight(Math.max(120, Math.min(maxHeight(), value)));
  }
  function destination(
    event: DragEvent<HTMLElement>,
    group: TerminalGroup,
    element: HTMLElement,
  ) {
    const rect = element.getBoundingClientRect();
    const tab = (event.target as HTMLElement).closest<HTMLElement>(
      "[data-terminal-tab]",
    );
    if (tab) {
      const bounds = tab.getBoundingClientRect();
      return {
        group: group.id,
        edge: "center" as const,
        index:
          group.tabs.indexOf(tab.dataset.terminalTab!) +
          (event.clientX > bounds.left + bounds.width / 2 ? 1 : 0),
      };
    }
    const header = element.querySelector("header")!.getBoundingClientRect();
    if (event.clientY < header.bottom)
      return { group: group.id, edge: "center" as const };
    const x = (event.clientX - rect.left) / rect.width;
    const y =
      (event.clientY - header.bottom) /
      Math.max(1, rect.bottom - header.bottom);
    const distances: [DockEdge, number][] = [
      ["left", x],
      ["right", 1 - x],
      ["top", y],
      ["bottom", 1 - y],
    ];
    const nearest = distances.sort((a, b) => a[1] - b[1])[0]!;
    return {
      group: group.id,
      edge: nearest[1] < 0.25 ? nearest[0] : ("center" as DockEdge),
    };
  }
  function renderGroup(groupId: string) {
    const group = layout.groups.find((group) => group.id === groupId)!;
    const status = statuses[group.active];
    const tabId = (id: string) => `${prefix}-${id}`;
    const paneId = `${prefix}-${groupId}`;
    return (
      <section
        className={styles.group}
        data-terminal-group={groupId}
        data-focused={group.tabs.includes(focused)}
      >
        <header className={styles.header}>
          <div className={styles.tabs}>
            <div
              className={styles.tabList}
              role="tablist"
              aria-label={t("Terminal tabs")}
            >
              {group.tabs.map((id) => (
                <div
                  key={id}
                  className={styles.tabItem}
                  data-selected={group.active === id}
                  data-terminal-tab={id}
                  draggable
                  onDragStart={(event) => {
                    event.dataTransfer.setData(dragType, id);
                    event.dataTransfer.effectAllowed = "move";
                    dragging.current = id;
                  }}
                  onDragEnd={() => {
                    dragging.current = null;
                    setDrop(null);
                  }}
                >
                  <Button
                    className={styles.tab}
                    role="tab"
                    id={tabId(id)}
                    aria-selected={group.active === id}
                    aria-controls={paneId}
                    tabIndex={group.active === id ? 0 : -1}
                    onClick={() => activate(id)}
                    onKeyDown={(event) => {
                      if (event.key === "Delete") {
                        event.preventDefault();
                        close(id);
                        return;
                      }
                      if (
                        !["ArrowLeft", "ArrowRight", "Home", "End"].includes(
                          event.key,
                        )
                      )
                        return;
                      event.preventDefault();
                      const offset = event.key === "ArrowLeft" ? -1 : 1;
                      const next =
                        group.tabs[
                          event.key === "Home"
                            ? 0
                            : event.key === "End"
                              ? group.tabs.length - 1
                              : (group.tabs.indexOf(id) +
                                  offset +
                                  group.tabs.length) %
                                group.tabs.length
                        ]!;
                      setLayout((current) => selectTerminal(current, next));
                      setFocused("");
                      document.getElementById(tabId(next))?.focus();
                    }}
                  >
                    {name(id)}
                  </Button>
                  <Button
                    variant="toolbar"
                    className={styles.close}
                    aria-label={t("Close {name}", { name: name(id) })}
                    title={t("Close {name}", { name: name(id) })}
                    onClick={() => close(id)}
                  >
                    ×
                  </Button>
                </div>
              ))}
            </div>
            <Button
              variant="toolbar"
              className={styles.control}
              aria-label={t("New terminal")}
              title={t("New terminal")}
              onClick={() => add(groupId)}
            >
              +
            </Button>
          </div>
          {groupId === order[0] && (
            <Button
              variant="toolbar"
              className={styles.fold}
              aria-label={t("Hide terminal")}
              title={hideShortcut}
              onClick={hide}
            >
              ⌄
            </Button>
          )}
        </header>
        <div className={styles.feedback}>
          {(error || status?.state) && (
            <small
              className={styles.status}
              role={error || status?.ended ? "alert" : "status"}
            >
              {error || formatStatus(status?.state ?? "")}
            </small>
          )}
          <span
            className={`terminal-clipboard-feedback ${styles.clipboardFeedback}`}
            role="status"
            aria-live="polite"
            title={
              status?.clipboardNotice === "failed"
                ? t(
                    "Clipboard access failed. Use the terminal context menu to copy.",
                  )
                : undefined
            }
          >
            {status?.clipboardNotice === "copied"
              ? t("Copied")
              : status?.clipboardNotice === "failed"
                ? t("Copy failed")
                : ""}
          </span>
          {status?.ended && (
            <Button
              className={styles.action}
              onClick={() => handles.current.get(group.active)?.reconnect()}
            >
              {t("Reconnect")}
            </Button>
          )}
        </div>
        <div
          className={styles.slot}
          ref={(node) => {
            if (node) slots.current.set(groupId, node);
            else slots.current.delete(groupId);
          }}
          id={paneId}
          role="tabpanel"
          aria-labelledby={tabId(group.active)}
          data-terminal-slot={groupId}
        />
        {drop?.group === groupId && (
          <div
            className={styles.dropPreview}
            data-edge={drop.edge}
            aria-hidden="true"
          />
        )}
      </section>
    );
  }
  return (
    <section
      className={`${styles.panel} ${className}`}
      ref={panel}
      aria-label={t("Workspace terminal")}
      hidden={!visible}
      style={height === undefined ? undefined : { height }}
      onDragOver={(event) => {
        if (!dragging.current || !event.dataTransfer.types.includes(dragType))
          return;
        event.preventDefault();
        event.dataTransfer.dropEffect = "move";
        // Portal events bubble through the shell's React owner, not its DOM pane.
        const element = (event.target as HTMLElement).closest<HTMLElement>(
          "[data-terminal-group]",
        );
        const group = layout.groups.find(
          (item) => item.id === element?.dataset.terminalGroup,
        );
        if (!element || !group) return;
        const target = destination(event, group, element);
        setDrop((current) =>
          current?.group === target.group &&
          current.edge === target.edge &&
          current.index === target.index
            ? current
            : target,
        );
      }}
      onDragLeave={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null))
          setDrop(null);
      }}
      onDrop={(event) => {
        if (!dragging.current) return;
        event.preventDefault();
        // Portal events bubble through the shell's React owner, not its DOM pane.
        const element = (event.target as HTMLElement).closest<HTMLElement>(
          "[data-terminal-group]",
        );
        const group = layout.groups.find(
          (item) => item.id === element?.dataset.terminalGroup,
        );
        if (!element || !group) return;
        const target = destination(event, group, element);
        const id = dragging.current;
        const newGroupId = `group-dock-${nextDock.current++}`;
        setLayout((current) =>
          moveTerminal(
            current,
            id,
            target.group,
            target.edge,
            newGroupId,
            target.index,
          ),
        );
        setFocused(id);
        requestFocus((value) => value + 1);
        dragging.current = null;
        setDrop(null);
      }}
    >
      <div
        className={styles.heightHandle}
        role="separator"
        tabIndex={0}
        aria-label={t("Resize terminal")}
        aria-orientation="horizontal"
        aria-valuemin={120}
        aria-valuenow={
          height ?? Math.min(300, Math.max(160, window.innerHeight * 0.28))
        }
        onPointerDown={(event) => {
          if (event.button !== 0) return;
          event.preventDefault();
          resizeOrigin.current = {
            y: event.clientY,
            height: panel.current!.clientHeight,
          };
          event.currentTarget.setPointerCapture(event.pointerId);
        }}
        onPointerMove={(event) => {
          if (event.currentTarget.hasPointerCapture(event.pointerId))
            resize(
              resizeOrigin.current.height +
                resizeOrigin.current.y -
                event.clientY,
            );
        }}
        onPointerUp={(event) => {
          if (event.currentTarget.hasPointerCapture(event.pointerId))
            event.currentTarget.releasePointerCapture(event.pointerId);
        }}
        onKeyDown={(event) => {
          if (["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) {
            event.preventDefault();
            resize(
              event.key === "Home"
                ? 120
                : event.key === "End"
                  ? maxHeight()
                  : panel.current!.clientHeight +
                    (event.key === "ArrowUp" ? 16 : -16),
            );
          }
        }}
      />
      {layout.tree && (
        <SplitLayout
          node={layout.tree}
          renderPane={renderGroup}
          separatorLabel={t("Resize panes")}
          onResize={(id, ratio) =>
            setLayout((current) =>
              current.tree
                ? { ...current, tree: setSplitRatio(current.tree, id, ratio) }
                : current,
            )
          }
        />
      )}
      {sessions.map((session) => (
        <Fragment key={session.id}>
          {renderSession({
            ...session,
            focused: focused === session.id,
            visible,
            placement: layout,
            getHost,
            onStatus,
            onFocus: setFocused,
            ref: (handle) => {
              if (handle) handles.current.set(session.id, handle);
              else handles.current.delete(session.id);
            },
          })}
        </Fragment>
      ))}
    </section>
  );
}
