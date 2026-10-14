import { ResourceValue as ValueMenu } from "#src/shared/components/resource-value.tsx";
import { useEffect, useId, useRef, useState } from "react";
import { Button, DialogCard, DialogActions } from "@lesomnus/cxz-ui";
import { useNavigate } from "@tanstack/react-router";
import { requiredSessionLogin } from "#src/features/accounts/login-required.ts";
import type { Project } from "#gen/cxz/project_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { listAccounts } from "#src/features/accounts/account-data.ts";
import {
  LoginPanel,
  type LoginTarget,
} from "#src/features/accounts/login-panel.tsx";
import { AnchoredDialog } from "#src/shared/components/anchored-dialog.tsx";
import styles from "#src/shared/components/resource-surface.module.css";

export function SessionCreate({
  c,
  projects,
  anchor,
  trigger,
  close,
}: {
  c: Connection;
  projects: Project[];
  anchor?: HTMLElement | null;
  trigger?: HTMLElement | null;
  close: () => void;
}) {
  const navigate = useNavigate();
  const formId = useId();
  const pending = useRef(false);
  const request = useRef<AbortController | undefined>(undefined);
  useEffect(() => () => request.current?.abort(), []);
  const accounts = useRemote((signal) => listAccounts(c, signal), [c]);
  const [project, setProject] = useState(projects[0]?.runtimeId ?? "");
  const [alias, setAlias] = useState("");
  const [title, setTitle] = useState("");
  const [model, setModel] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [login, setLogin] = useState<LoginTarget>();
  const creation = useRef({ signature: "", id: "" });
  const selected = accounts.data?.find((a) => a.alias === alias);
  useEffect(() => {
    if (!project && projects[0]) setProject(projects[0].runtimeId);
  }, [projects, project]);
  async function createSession(afterLogin = false) {
    if (!selected || !project || pending.current || (login && !afterLogin))
      return;
    pending.current = true;
    const controller = new AbortController();
    request.current = controller;
    const signature = JSON.stringify([project, alias, title, model]);
    if (creation.current.signature !== signature)
      creation.current = { signature, id: crypto.randomUUID() };
    const clientId = creation.current.id;
    setBusy(true);
    setError("");
    try {
      const account = { key: { case: "alias" as const, value: alias } };
      const binding = await c.bindings.add(
        { project: ref(project), account },
        { signal: controller.signal, timeoutMs: 10000 },
      );
      const session = await c.sessions.add(
        {
          project: ref(project),
          account,
          authBinding: { key: { case: "bindingId", value: binding.bindingId } },
          name: title,
          model,
          clientId,
        },
        { signal: controller.signal, timeoutMs: 120000 },
      );
      if (controller.signal.aborted) return;
      c.store.apply("cxz.Session", [{ id: session.id, value: session }]);
      close();
      await navigate({
        to: "/sessions/$sessionId",
        params: { sessionId: session.runtimeId },
      });
    } catch (error) {
      if (controller.signal.aborted) return;
      // Missing login is the only failed creation we retry, with the SAME key.
      if (
        !afterLogin &&
        selected.agent === "claude" &&
        requiredSessionLogin(error, alias, clientId)
      )
        setLogin({ account: alias, project, key: clientId });
      else setError(String(error));
    } finally {
      pending.current = false;
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return (
    <AnchoredDialog
      title="Add session"
      anchor={anchor}
      trigger={trigger}
      close={close}
      busy={busy}
    >
      <form
        id={formId}
        onSubmit={(event) => {
          event.preventDefault();
          void createSession();
        }}
      >
        <DialogCard
          title="Add session"
          close={busy ? undefined : close}
          closeLabel="Close"
          footer={
            <DialogActions>
              <Button
                type="submit"
                form={formId}
                disabled={busy || !project || !selected || !!login}
              >
                {busy ? "Creating…" : "Create session"}
              </Button>
              <Button type="button" disabled={busy} onClick={close}>
                Cancel
              </Button>
            </DialogActions>
          }
        >
          <div className={styles.list}>
            <ValueMenu
              label="Project"
              value={project}
              options={projects.map((p) => ({
                value: p.runtimeId,
                label: p.name || p.alias,
              }))}
              choose={setProject}
              disabled={busy || !!login}
            />
            <ValueMenu
              label="Account"
              value={alias}
              options={[
                { value: "", label: "Choose an account" },
                ...(accounts.data ?? []).map((a) => ({
                  value: a.alias,
                  label: `${a.name || a.alias} · ${a.agent}`,
                })),
              ]}
              choose={setAlias}
              disabled={busy || !!login}
            />
            <label className={styles.field}>
              Title
              <input
                autoFocus
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                disabled={busy || !!login}
              />
            </label>
            <label className={styles.field}>
              Model
              <input
                value={model}
                onChange={(e) => setModel(e.target.value)}
                placeholder="Provider default"
                disabled={busy || !!login}
              />
            </label>
            {(error || accounts.error) && (
              <p role="alert">{error || accounts.error}</p>
            )}
            {login && (
              <LoginPanel
                c={c}
                target={login}
                complete={() => {
                  setLogin(undefined);
                  void createSession(true);
                }}
                cancel={() => setLogin(undefined)}
              />
            )}
          </div>
        </DialogCard>
      </form>
    </AnchoredDialog>
  );
}
