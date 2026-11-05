import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useEffect, useRef, useState, useId } from "react";
import {
  Button,
  Card,
  CardActions,
  Form,
  FormField,
  ValueMenu,
} from "@lesomnus/nox";
import type { Account } from "#gen/cxz/account_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { AnchoredDialog } from "#src/shared/components/anchored-dialog.tsx";
import { useDialogClose } from "#src/shared/components/dialog-close.ts";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
export function AccountAddForm({
  c,
  anchor,
  added,
  close,
  trigger,
}: {
  c: Connection;
  anchor?: HTMLElement | null;
  added: (account: Account) => void;
  close: () => void;
  trigger?: HTMLElement | null;
}) {
  const dialogClose = useDialogClose(close);
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
      open={dialogClose.open}
      close={dialogClose.dismiss}
      onClosed={dialogClose.onClosed}
      busy={busy}
      trigger={trigger}
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
            if (!controller.signal.aborted)
              dialogClose.complete(() => added(account));
          } catch (error) {
            if (!controller.signal.aborted) setError(String(error));
          } finally {
            pending.current = false;
            if (!controller.signal.aborted) setBusy(false);
          }
        }}
      >
        <Card
          title="Add account"
          close={busy ? undefined : dialogClose.dismiss}
          closeLabel="Close"
          data-entered="true"
          footer={
            <CardActions>
              <Button
                type="submit"
                form={id}
                loading={busy}
                disabled={busy || !/^[a-z][a-z0-9-]{0,62}$/.test(alias.trim())}
              >
                Confirm
              </Button>
              <Button
                type="button"
                disabled={busy}
                onClick={dialogClose.dismiss}
              >
                Cancel
              </Button>
            </CardActions>
          }
        >
          <Form
            as="fieldset"
            columns={1}
            disabled={busy}
            aria-label="Account details"
          >
            <FormField label="Agent">
              <ValueMenu
                label="Agent"
                alignItemWithTrigger
                value={agent}
                options={[
                  {
                    value: "codex",
                    label: "Codex",
                    icon: <AgentBrand agent="codex" />,
                  },
                  {
                    value: "claude",
                    label: "Claude",
                    icon: <AgentBrand agent="claude" />,
                  },
                ]}
                choose={setAgent}
                disabled={busy}
              />
            </FormField>
            <FormField label="Alias" htmlFor={`${id}-alias`}>
              <input
                id={`${id}-alias`}
                autoFocus
                required
                pattern="[a-z][a-z0-9-]{0,62}"
                maxLength={63}
                value={alias}
                onChange={(event) => setAlias(event.target.value)}
                autoComplete="off"
              />
            </FormField>
            <FormField label="Display name" htmlFor={`${id}-name`}>
              <input
                id={`${id}-name`}
                maxLength={200}
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </FormField>
            <small className={styles.muted}>
              {agent === "codex"
                ? "Codex uses a central login shared by this account's sessions."
                : "Claude uses an independent login for each session."}
            </small>
            {error && <ErrorNotice error={error} />}
          </Form>
        </Card>
      </form>
    </AnchoredDialog>
  );
}
