import { useEffect, useRef, useState, useId } from "react";
import { Button, DialogCard, DialogActions } from "@lesomnus/cxz-ui";
import type { Account } from "#gen/cxz/account_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { AnchoredDialog } from "#src/shared/components/anchored-dialog.tsx";
import { ResourceValue } from "#src/shared/components/resource-value.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
import accountStyles from "./accounts.module.css";
export function AccountAddForm({
  c,
  anchor,
  added,
  close,
}: {
  c: Connection;
  anchor?: HTMLElement | null;
  added: (account: Account) => void;
  close: () => void;
}) {
  const [agent, setAgent] = useState("codex"),
    [alias, setAlias] = useState(""),
    [name, setName] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const pending = useRef(false),
    request = useRef<AbortController | undefined>(undefined);
  const id = useId();
  useEffect(() => () => request.current?.abort(), []);
  return (
    <AnchoredDialog
      title="Add account"
      anchor={anchor}
      close={close}
      busy={busy}
    >
      <form
        id={id}
        onSubmit={async (event) => {
          event.preventDefault();
          if (pending.current) return;
          pending.current = true;
          setBusy(true);
          setError("");
          const controller = new AbortController();
          request.current = controller;
          try {
            const account = await c.accounts.add(
              { agent, alias: alias.trim(), name: name.trim() },
              { signal: controller.signal, timeoutMs: 30000 },
            );
            if (!controller.signal.aborted) added(account);
          } catch (error) {
            if (!controller.signal.aborted) setError(String(error));
          } finally {
            pending.current = false;
            if (!controller.signal.aborted) setBusy(false);
          }
        }}
      >
        <DialogCard
          title="Add account"
          close={busy ? undefined : close}
          closeLabel="Close"
          data-entered="true"
          footer={
            <DialogActions>
              <Button
                type="submit"
                form={id}
                disabled={busy || !/^[a-z][a-z0-9-]{0,62}$/.test(alias.trim())}
              >
                {busy ? "Adding…" : "Add"}
              </Button>
              <Button type="button" disabled={busy} onClick={close}>
                Cancel
              </Button>
            </DialogActions>
          }
        >
          <fieldset disabled={busy} className={accountStyles.form}>
            <ResourceValue
              label="Agent"
              value={agent}
              options={[
                { value: "codex", label: "Codex" },
                { value: "claude", label: "Claude" },
              ]}
              choose={setAgent}
              disabled={busy}
            />
            <label className={styles.field}>
              Alias
              <input
                autoFocus
                required
                pattern="[a-z][a-z0-9-]{0,62}"
                maxLength={63}
                value={alias}
                onChange={(event) => setAlias(event.target.value)}
                autoComplete="off"
              />
            </label>
            <label className={styles.field}>
              Display name
              <input
                maxLength={200}
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            <small className={styles.muted}>
              {agent === "codex"
                ? "Codex uses a central login shared by this account's sessions."
                : "Claude uses an independent login for each session."}
            </small>
            {error && <p role="alert">{error}</p>}
          </fieldset>
        </DialogCard>
      </form>
    </AnchoredDialog>
  );
}
