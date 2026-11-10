import {
  cloneElement,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type CSSProperties,
  type ReactElement,
} from "react";
import {
  CompactNumber,
  MenuIcon,
  MenuItemLabel,
  MenuScrollArea,
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

export function SidebarAccounts({
  c,
  sessions,
  children,
}: {
  c: Connection;
  sessions: Session[];
  children: ReactElement<{
    "aria-haspopup"?: "dialog";
    "aria-expanded"?: boolean;
    "aria-controls"?: string;
    "data-menu-open"?: boolean;
  }>;
}) {
  const id = useId();
  const root = useRef<HTMLDivElement>(null);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const [anchor, setAnchor] = useState<HTMLAnchorElement | null>(null);
  const cancelClose = useCallback(() => clearTimeout(timer.current), []);
  const close = useCallback((restoreFocus = false) => {
    clearTimeout(timer.current);
    if (restoreFocus)
      root.current?.querySelector("a")?.focus({ preventScroll: true });
    setAnchor(null);
  }, []);
  useEffect(() => cancelClose, [cancelClose]);
  const show = () => {
    cancelClose();
    setAnchor(root.current?.querySelector("a") ?? null);
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
        if ((event.target as Element).closest("a")) close();
      }}
    >
      {cloneElement(children, {
        "aria-haspopup": "dialog",
        "aria-expanded": !!anchor,
        "aria-controls": anchor ? id : undefined,
        "data-menu-open": !!anchor,
      })}
      {anchor && (
        <AnchoredMenu
          id={id}
          anchor={anchor}
          label="Account usage"
          placement="right"
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
  return (
    <MenuScrollArea aria-busy={accounts.loading}>
      <div role="list" aria-label="Accounts">
        {accounts.data?.map((account) => (
          <AccountItem
            key={key(account.id) || account.alias}
            c={c}
            account={account}
            sessions={sessionsForAccount(account, sessions)}
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
}: {
  c: Connection;
  account: Account;
  sessions: Session[];
}) {
  const quota = useAccountQuota({
    c,
    agent: account.agent,
    sessions,
    identity: account.id.length ? key(account.id) : account.alias,
  });
  const remaining = quota.data?.remaining;
  return (
    <div
      role="listitem"
      className={styles.item}
      style={
        {
          "--usage-fill": `${Math.max(0, Math.min(100, remaining ?? 0))}%`,
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
        {remaining === undefined ? (
          <span className={styles.unit}>{quota.loading ? "--" : "—"}</span>
        ) : (
          <>
            <CompactNumber
              mode="integer"
              digits={3}
              value={Math.round(remaining)}
            />
            <span className={styles.unit}>%</span>
          </>
        )}
      </span>
    </div>
  );
}
