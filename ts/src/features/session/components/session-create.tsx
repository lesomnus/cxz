import { ResourceValue as ValueMenu } from "#src/shared/components/resource-value.tsx";
import { useEffect, useRef, useState } from "react";
import { Button } from "@lesomnus/cxz-ui";
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
import styles from "#src/shared/components/resource-surface.module.css";

export function SessionCreate({
  c,
  projects,
}: {
  c: Connection;
  projects: Project[];
}) {
  const navigate = useNavigate();
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
    if (!selected || !project || busy) return;
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
        { timeoutMs: 10000 },
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
        { timeoutMs: 120000 },
      );
      c.store.apply("cxz.Session", [{ id: session.id, value: session }]);
      await navigate({
        to: "/sessions/$sessionId",
        params: { sessionId: session.runtimeId },
      });
    } catch (error) {
      // Missing login is the only failed creation we retry, with the SAME key.
      if (
        !afterLogin &&
        selected.agent === "claude" &&
        requiredSessionLogin(error, alias, clientId)
      )
        setLogin({ account: alias, project, key: clientId });
      else setError(String(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className={styles.section} aria-label="New session">
      <strong>New session</strong>
      <ValueMenu
        label="Project"
        searchable
        value={project}
        options={projects.map((p) => ({
          value: p.runtimeId,
          label: p.name || p.alias,
          searchText: p.alias,
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
      <Button
        disabled={busy || !project || !selected || !!login}
        onClick={() => void createSession()}
      >
        Create session
      </Button>
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
    </section>
  );
}
