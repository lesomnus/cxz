import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { sessionLocation } from "#src/shared/navigation/resource-route.ts";
import { projectId, projectRef } from "#src/shared/api/project-id.ts";
import { useEffect, useId, useRef, useState } from "react";
import {
  Button,
  Card,
  CardActions,
  Form,
  FormField,
  ValueMenu,
} from "@lesomnus/nox";
import { useNavigate } from "@tanstack/react-router";
import { requiredSessionLogin } from "#src/features/accounts/login-required.ts";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { listAccounts } from "#src/features/accounts/account-data.ts";
import {
  LoginPanel,
  type LoginTarget,
} from "#src/features/accounts/login-panel.tsx";
import { AnchoredDialog } from "#src/shared/components/anchored-dialog.tsx";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { useDialogClose } from "#src/shared/components/dialog-close.ts";
import styles from "#src/shared/components/resource-surface.module.css";

export function SessionCreate({
  c,
  projects,
  initialProject,
  fixedProject,
  anchor,
  trigger,
  centered = true,
  close,
}: {
  c: Connection;
  projects: Project[];
  initialProject?: string;
  fixedProject?: string;
  anchor?: HTMLElement | null;
  trigger?: HTMLElement | null;
  centered?: boolean;
  close: () => void;
}) {
  const dialogClose = useDialogClose(close);
  const navigate = useNavigate();
  const formId = useId();
  const pending = useRef(false);
  const request = useRef<AbortController | undefined>(undefined);
  useEffect(() => () => request.current?.abort(), []);
  const accounts = useRemote((signal) => listAccounts(c, signal), [c]);
  const defaultProject =
    projects.find((p) => projectId(p) === initialProject) ?? projects[0];
  const [chosenProject, setProject] = useState(() => projectId(defaultProject));
  const project = fixedProject ?? chosenProject;
  const [alias, setAlias] = useState("");
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<Session>();
  const [error, setError] = useState("");
  const [login, setLogin] = useState<LoginTarget>();
  const creation = useRef({ signature: "", id: "" });
  const selected = accounts.data?.find((a) => a.alias === alias);
  useEffect(() => {
    if (!project && defaultProject) setProject(projectId(defaultProject));
  }, [defaultProject, project]);
  async function createSession(afterLogin = false) {
    if (!selected || !project || pending.current || (login && !afterLogin))
      return;
    pending.current = true;
    const controller = new AbortController();
    request.current = controller;
    const signature = JSON.stringify([project, alias, title]);
    if (creation.current.signature !== signature)
      creation.current = { signature, id: crypto.randomUUID() };
    const clientId = creation.current.id;
    setBusy(true);
    setError("");
    try {
      const account = { key: { case: "alias" as const, value: alias } };
      const binding = created
        ? undefined
        : await c.bindings.add(
            { project: projectRef(project), account },
            { signal: controller.signal, timeoutMs: 10000 },
          );
      const session =
        created ??
        (await c.sessions.add(
          {
            project: projectRef(project),
            account,
            authBinding: {
              key: { case: "bindingId", value: binding!.bindingId },
            },
            name: title,
            clientId,
          },
          { signal: controller.signal, timeoutMs: 10000 },
        ));
      if (controller.signal.aborted) return;
      setCreated(session);
      c.store.apply("cxz.Session", [{ id: session.id, value: session }]);
      const started = await c.sessions.start(
        {
          ref: ref(session.runtimeId),
          runId: session.status?.runId ?? "",
          clientId: crypto.randomUUID(),
        },
        { signal: controller.signal, timeoutMs: 120000 },
      );
      if (controller.signal.aborted) return;
      c.store.apply("cxz.Session", [{ id: started.id, value: started }]);
      dialogClose.complete(() => {
        close();
        void navigate({ ...sessionLocation(started) });
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
      centered={centered}
      trigger={trigger}
      open={dialogClose.open}
      close={dialogClose.dismiss}
      onClosed={dialogClose.onClosed}
      busy={busy}
    >
      <form
        id={formId}
        onSubmit={(event) => {
          event.preventDefault();
          void createSession();
        }}
      >
        <Card
          title="Add session"
          close={dialogClose.dismiss}
          closeDisabled={busy}
          closeLabel="Close"
          footer={
            <CardActions>
              <Button
                type="submit"
                form={formId}
                loading={busy}
                disabled={busy || !project || !selected || !!login}
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
          <div className={styles.list}>
            <Form
              as="fieldset"
              columns={1}
              disabled={busy || !!login || !!created}
              aria-label="Session details"
            >
              {fixedProject === undefined && (
                <FormField label="Project">
                  <ValueMenu
                    label="Project"
                    searchable
                    value={project}
                    options={projects.map((p) => ({
                      value: projectId(p),
                      label: p.name || p.alias,
                      searchText: p.alias,
                    }))}
                    choose={setProject}
                    disabled={busy || !!login || !!created}
                  />
                </FormField>
              )}
              <FormField label="Account">
                <ValueMenu
                  label="Account"
                  reserveDescriptionSpace
                  alignItemWithTrigger
                  value={alias}
                  options={[
                    { value: "", label: "Choose an account" },
                    ...(accounts.data ?? []).map((a) => ({
                      value: a.alias,
                      label: a.name || a.alias,
                      description: a.alias,
                      icon: <AgentBrand agent={a.agent} />,
                    })),
                  ]}
                  choose={setAlias}
                  disabled={busy || !!login || !!created}
                />
              </FormField>
              <FormField label="Title" htmlFor={`${formId}-title`}>
                <input
                  id={`${formId}-title`}
                  autoFocus
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  disabled={busy || !!login || !!created}
                />
              </FormField>
            </Form>
            {(error || accounts.error) && (
              <ErrorNotice error={error || accounts.error} />
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
        </Card>
      </form>
    </AnchoredDialog>
  );
}
