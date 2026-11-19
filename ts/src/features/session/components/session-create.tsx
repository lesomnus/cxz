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
import type { Project } from "#gen/cxz/project_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { listAccounts } from "#src/features/accounts/account-data.ts";
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
  const [error, setError] = useState("");
  const creation = useRef({ signature: "", id: "" });
  const selected = accounts.data?.find((a) => a.alias === alias);
  useEffect(() => {
    if (!project && defaultProject) setProject(projectId(defaultProject));
  }, [defaultProject, project]);
  async function createSession() {
    if (!selected || !project || pending.current || !dialogClose.open) return;
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
      const ref = projectRef(project);
      const options = { signal: controller.signal, timeoutMs: 10000 };
      const binding = await c.bindings.add({ project: ref, account }, options);
      const session = await c.sessions.add(
        {
          project: ref,
          account,
          authBinding: {
            key: { case: "bindingId", value: binding.bindingId },
          },
          name: title,
          clientId,
        },
        options,
      );
      if (controller.signal.aborted) return;
      c.store.apply("cxz.Session", [{ id: session.id, value: session }]);
      dialogClose.complete(() => {
        c.pendingSessionStarts.add(session.runtimeId);
        close();
        void navigate(sessionLocation(session));
      });
    } catch (error) {
      if (controller.signal.aborted) return;
      setError(String(error));
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
                disabled={busy || !project || !selected}
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
              disabled={busy}
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
                    disabled={busy}
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
                  disabled={busy}
                />
              </FormField>
              <FormField label="Title" htmlFor={`${formId}-title`}>
                <input
                  id={`${formId}-title`}
                  autoFocus
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  disabled={busy}
                />
              </FormField>
            </Form>
            {(error || accounts.error) && (
              <ErrorNotice error={error || accounts.error} />
            )}
          </div>
        </Card>
      </form>
    </AnchoredDialog>
  );
}
