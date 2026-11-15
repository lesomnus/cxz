import {
  cloneElement,
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactElement,
  type ReactNode,
} from "react";
import {
  CompactNumber,
  DurationDisplay,
  MenuIcon,
  MenuItemLabel,
  MenuScrollArea,
  useValueMotion,
} from "@lesomnus/nox";
import { key } from "@lesomnus/payday/store";
import type { Account } from "#gen/cxz/account_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import {
  ACCOUNT_INFO_TTL,
  ACCOUNT_LIST_KEY,
} from "#src/shared/api/read-cache.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { AnchoredMenu } from "#src/shared/components/anchored-menu.tsx";
import { listAccounts, sessionsForAccount } from "./account-data";
import { useAccountQuota } from "./use-account-quota";
import styles from "./sidebar-accounts.module.css";
import { sidebarPopupGap } from "#src/shared/navigation/sidebar-popup.ts";

const entranceCooldown = 60_000;
const entranceTimes = new WeakMap<Connection, Map<string, number>>();
function recentEntrance(c: Connection, identity: string) {
  return (
    Date.now() - (entranceTimes.get(c)?.get(identity) ?? -Infinity) <
    entranceCooldown
  );
}
function rememberEntrance(c: Connection, identity: string) {
  let times = entranceTimes.get(c);
  if (!times) entranceTimes.set(c, (times = new Map()));
  times.delete(identity);
  times.set(identity, Date.now());
  if (times.size > 256) times.delete(times.keys().next().value!);
}

export function SidebarAccounts({
  c,
  sessions,
  children,
  enabled = true,
}: {
  c: Connection;
  sessions: Session[];
  enabled?: boolean;
  children:
    | ReactElement<{
        "aria-haspopup"?: "dialog";
        "aria-expanded"?: boolean;
        "aria-controls"?: string;
        "data-menu-open"?: boolean;
      }>
    | ((open: boolean) => ReactNode);
}) {
  const id = useId();
  const root = useRef<HTMLDivElement>(null);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const cancelClose = useCallback(() => clearTimeout(timer.current), []);
  const close = useCallback((restoreFocus = false) => {
    clearTimeout(timer.current);
    if (restoreFocus)
      root.current
        ?.querySelector<HTMLElement>("a, button")
        ?.focus({ preventScroll: true });
    setAnchor(null);
  }, []);
  useEffect(() => cancelClose, [cancelClose]);
  useEffect(() => {
    if (!enabled) close();
  }, [enabled, close]);
  const show = () => {
    if (!enabled) return;
    cancelClose();
    setAnchor(root.current?.querySelector<HTMLElement>("a, button") ?? null);
  };
  return (
    <div
      ref={root}
      className={styles.trigger}
      onPointerEnter={(event) => {
        if (event.pointerType !== "touch") show();
      }}
      onPointerLeave={() => {
        cancelClose();
        timer.current = setTimeout(() => close(), 160);
      }}
      onFocus={show}
      onBlur={(event) => {
        if (
          !root.current?.contains(event.relatedTarget) &&
          !document.getElementById(id)?.contains(event.relatedTarget)
        )
          close();
      }}
      onClick={(event) => {
        if ((event.target as Element).closest("a, button")) close();
      }}
    >
      {typeof children === "function"
        ? children(enabled && !!anchor)
        : cloneElement(children, {
            "aria-haspopup": "dialog",
            "aria-expanded": !!anchor,
            "aria-controls": anchor ? id : undefined,
            "data-menu-open": !!anchor,
          })}
      {enabled && anchor && (
        <AnchoredMenu
          id={id}
          anchor={anchor}
          label="Account usage"
          placement="right"
          gap={sidebarPopupGap}
          autoFocus={false}
          className={styles.preview}
          role="dialog"
          close={close}
        >
          <AccountItems c={c} sessions={sessions} />
        </AnchoredMenu>
      )}
    </div>
  );
}

function AccountItems({ c, sessions }: { c: Connection; sessions: Session[] }) {
  const accounts = useRemote((signal) => listAccounts(c, signal), [c], {
    cache: { store: c.reads, key: ACCOUNT_LIST_KEY, ttl: ACCOUNT_INFO_TTL },
  });
  const [ready, setReady] = useState<ReadonlyMap<string, boolean>>(
    () => new Map(),
  );
  const [revealed, setRevealed] = useState<ReadonlySet<string>>(
    () => new Set(),
  );
  const lastStart = useRef(-Infinity);
  const markReady = useCallback((identity: string, skipEntrance: boolean) => {
    setReady((current) =>
      current.has(identity)
        ? current
        : new Map(current).set(identity, skipEntrance),
    );
  }, []);
  useEffect(() => {
    const next = accounts.data?.find(
      (account) => !revealed.has(key(account.id) || account.alias),
    );
    if (!next) return;
    const identity = key(next.id) || next.alias;
    if (!ready.has(identity)) return;
    if (ready.get(identity)) {
      setRevealed((current) => new Set(current).add(identity));
      return;
    }
    // Metadata may arrive out of order; stagger starts in the displayed order.
    const timer = setTimeout(
      () => {
        lastStart.current = performance.now();
        setRevealed((current) => new Set(current).add(identity));
      },
      Math.max(0, lastStart.current + 120 - performance.now()),
    );
    return () => clearTimeout(timer);
  }, [accounts.data, ready, revealed]);
  return (
    <MenuScrollArea aria-busy={accounts.loading}>
      <div className={styles.list} role="list" aria-label="Accounts">
        {accounts.data?.map((account) => (
          <AccountItem
            key={key(account.id) || account.alias}
            c={c}
            account={account}
            sessions={sessionsForAccount(account, sessions)}
            revealed={revealed.has(key(account.id) || account.alias)}
            onReady={markReady}
          />
        ))}
      </div>
      {!accounts.data?.length && (
        <p className={styles.state} role="status">
          {accounts.loading
            ? "Loading accounts…"
            : accounts.error
              ? "Accounts unavailable"
              : "No accounts"}
        </p>
      )}
    </MenuScrollArea>
  );
}

function AccountItem({
  c,
  account,
  sessions,
  revealed,
  onReady,
}: {
  c: Connection;
  account: Account;
  sessions: Session[];
  revealed: boolean;
  onReady: (identity: string, skipEntrance: boolean) => void;
}) {
  const identity = account.id.length ? key(account.id) : account.alias;
  const quota = useAccountQuota({
    c,
    agent: account.agent,
    sessions,
    identity,
  });
  const remaining = quota.data?.remaining;
  const [skipEntrance] = useState(
    () => quota.fromCache || recentEntrance(c, identity),
  );
  const value = skipEntrance || revealed ? remaining : undefined;
  const [motion, setMotion] = useState(() => ({
    value,
    animate: false,
    duration: 0,
  }));
  if (motion.value !== value) {
    // Native transitions retarget from the currently rendered position, including mid-flight updates.
    setMotion({
      value,
      animate:
        value !== undefined && (motion.value !== undefined || !skipEntrance),
      duration:
        1200 * Math.sqrt(Math.abs((value ?? 0) - (motion.value ?? 0)) / 100),
    });
  }
  const numberMotion = useValueMotion({
    animateInitial: false,
    spatialDelay: false,
    characterJitter: 0,
    duration: motion.duration,
    shouldAnimate: () => motion.animate,
  });
  useLayoutEffect(() => {
    if (motion.animate) rememberEntrance(c, identity);
  }, [c, identity, motion]);
  useEffect(() => {
    if (remaining !== undefined || !quota.loading)
      onReady(identity, skipEntrance);
  }, [identity, remaining, quota.loading, onReady, skipEntrance]);
  const fraction = Math.max(0, Math.min(100, motion.value ?? 0)) / 100;
  return (
    <div
      role="listitem"
      className={styles.item}
      data-usage-visible={motion.value !== undefined}
      data-usage-animate={motion.animate}
      style={
        {
          "--usage-progress": fraction,
          // Constant acceleration makes duration proportional to sqrt(distance).
          "--usage-duration": `${motion.duration}ms`,
        } as CSSProperties
      }
    >
      <MenuIcon icon={<AgentBrand agent={account.agent} />} />
      <MenuItemLabel>{account.name || account.alias}</MenuItemLabel>
      <span
        className={styles.remaining}
        aria-label={
          remaining === undefined
            ? "Remaining usage unavailable"
            : `${Math.round(remaining)}% remaining`
        }
        aria-busy={quota.loading}
      >
        <CompactNumber
          {...numberMotion}
          mode="integer"
          digits={3}
          value={motion.value ?? NaN}
          className={
            motion.value === undefined ? styles.hiddenNumber : undefined
          }
        />
        {motion.value === undefined ? (
          <span className={styles.unit}>{quota.loading ? "--" : "—"}</span>
        ) : (
          <span className={`${styles.unit} ${styles.percent}`}>%</span>
        )}
      </span>
      <ResetTime reset={quota.data?.reset} />
    </div>
  );
}

function ResetTime({ reset }: { reset?: number }) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    setNow(Date.now());
    if (reset === undefined) return;
    const timer = setInterval(() => {
      if (document.visibilityState === "visible") setNow(Date.now());
    }, 1000);
    return () => clearInterval(timer);
  }, [reset]);
  return (
    <span
      className={styles.reset}
      role="timer"
      aria-label="Time until usage reset"
      aria-live="off"
    >
      <DurationDisplay
        durationMs={reset === undefined ? undefined : Math.max(0, reset - now)}
        rounding="ceil"
        label={reset === undefined ? "Usage reset unavailable" : undefined}
      />
    </span>
  );
}
