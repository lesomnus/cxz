import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import {
  Button,
  Dialog,
  DialogBody,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Form,
  FormField,
  ScrollArea,
  ValueMenu,
} from "@lesomnus/nox";
import { parseAlias } from "@lesomnus/payday/slug";
import type { Project } from "#gen/cxz/project_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useProjectConnection } from "#src/shared/api/project-connection.ts";
import { ConnectionIcon } from "#src/shared/components/connection-icon.tsx";
import { useDialogClose } from "#src/shared/components/dialog-close.ts";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useRouteConnections } from "#src/app/router.tsx";
import { validateProjectField } from "./project-edit";
import styles from "./project-add.module.css";

type ProjectDraft = {
  workspace: string;
  name: string;
  alias: string;
  config?: string;
};

const emptyDraft: ProjectDraft = { workspace: "", name: "", alias: "" };
const defaultKubeConfig = JSON.stringify(
  { image: "ghcr.io/lesomnus/dev-golang:1.27", user: "hypnos" },
  null,
  2,
);
const kubeName = /^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$/;

export function ProjectAdd({
  c,
  added,
  close,
}: {
  c: Connection;
  added: (project: Project, connection: string) => void;
  close: () => void;
}) {
  useLocale();
  const dialog = useDialogClose(close);
  const id = useId();
  const request = useRef<AbortController | undefined>(undefined);
  useEffect(() => () => request.current?.abort(), []);
  const [selected, setSelected] = useState<string>();
  const [draft, setDraft] = useState(emptyDraft);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const connections = useRouteConnections();
  const entry =
    connections.find((item) => item.handle === selected) ??
    connections.find((item) => item.baseUrl === c.baseUrl) ??
    connections[0];
  const target = entry && { ...entry, baseUrl: entry.baseUrl || c.baseUrl };
  const runtime = useProjectConnection(c, target?.baseUrl);
  const active =
    target && runtime.data?.baseUrl === target.baseUrl
      ? runtime.data
      : undefined;
  const backend = active?.backend;
  const docker = backend === "docker";
  const kubernetes = backend === "kubernetes";
  const config = draft.config ?? (kubernetes ? defaultKubeConfig : "");
  const workspace = draft.workspace.trim();
  const valid = !!active && !!workspace && (docker || kubernetes);
  const set = (field: keyof typeof emptyDraft, value: string) =>
    setDraft((old) => ({ ...old, [field]: value }));

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!valid || !active || !target || request.current || !dialog.open) return;
    setError("");
    if (kubernetes && !kubeName.test(workspace)) {
      setError(
        t(
          "Use 1–40 lowercase letters, digits or hyphens, starting and ending with a letter or digit.",
        ),
      );
      return;
    }
    const name = draft.name.trim();
    const alias = draft.alias.trim();
    const invalid =
      (name && validateProjectField("name", name)) ||
      (alias && validateProjectField("alias", alias));
    if (invalid) {
      setError(invalid);
      return;
    }
    const controller = new AbortController();
    request.current = controller;
    setBusy(true);
    try {
      const project = await active.projects.add(
        {
          workspace,
          name,
          alias: alias ? parseAlias(alias) : "",
          config: config.trim(),
        },
        { signal: controller.signal, timeoutMs: 30000 },
      );
      if (controller.signal.aborted) return;
      if (target.baseUrl === c.baseUrl)
        c.store.apply("cxz.Project", [{ id: project.id, value: project }]);
      dialog.complete(() => added(project, target.handle));
    } catch (error) {
      if (!controller.signal.aborted) setError(String(error));
    } finally {
      request.current = undefined;
      if (!controller.signal.aborted) setBusy(false);
    }
  }

  return (
    <Dialog
      open={dialog.open}
      onClose={dialog.dismiss}
      onClosed={dialog.onClosed}
      dismissible={!busy}
      aria-busy={busy}
    >
      <form className={styles.form} onSubmit={submit}>
        <DialogHeader>
          <DialogTitle>{t("Add project")}</DialogTitle>
        </DialogHeader>
        <DialogBody className={styles.body}>
          <ScrollArea
            className={styles.scroll}
            viewportClassName={styles.viewport}
            scrollbarLabel={t("Project details")}
          >
            <Form
              as="fieldset"
              columns={1}
              disabled={busy}
              aria-label={t("Project details")}
            >
              <FormField label={t("Connection")}>
                <ValueMenu
                  label={t("Connection")}
                  value={target?.handle ?? ""}
                  disabled={busy || !connections.length}
                  options={connections.map((item) => ({
                    value: item.handle,
                    label: item.alias,
                    icon: (
                      <ConnectionIcon
                        c={c}
                        transport={item.entry?.transport}
                        backend={
                          item.handle === target?.handle ? backend : undefined
                        }
                        baseUrl={
                          item.handle === target?.handle
                            ? undefined
                            : item.baseUrl
                        }
                      />
                    ),
                  }))}
                  choose={(value) => {
                    if (target?.handle === value) return;
                    setSelected(value);
                    setDraft(emptyDraft);
                    setError("");
                  }}
                />
              </FormField>
              {target && !active && runtime.loading && (
                <small role="status">{t("Loading…")}</small>
              )}
              {!active && runtime.error && (
                <div className={styles.error}>
                  <ErrorNotice error={runtime.error} local />
                  <Button type="button" onClick={runtime.reload}>
                    {t("Retry")}
                  </Button>
                </div>
              )}
              {active && !docker && !kubernetes && (
                <ErrorNotice
                  error={t(
                    backend
                      ? "Unsupported project runtime"
                      : "Update the connected manager to report its project runtime.",
                  )}
                />
              )}
              {(docker || kubernetes) && active && (
                <>
                  <FormField
                    label={t(docker ? "Workspace path" : "Project name")}
                    htmlFor={`${id}-workspace`}
                  >
                    <input
                      id={`${id}-workspace`}
                      required
                      autoComplete="off"
                      value={draft.workspace}
                      placeholder={
                        docker ? "/workspace/my-project" : "my-project"
                      }
                      onChange={(event) => set("workspace", event.target.value)}
                    />
                  </FormField>
                  <FormField label={t("Display name")} htmlFor={`${id}-name`}>
                    <input
                      id={`${id}-name`}
                      value={draft.name}
                      onChange={(event) => set("name", event.target.value)}
                    />
                  </FormField>
                  <FormField label={t("Alias")} htmlFor={`${id}-alias`}>
                    <input
                      id={`${id}-alias`}
                      autoComplete="off"
                      value={draft.alias}
                      onChange={(event) => set("alias", event.target.value)}
                    />
                  </FormField>
                  <FormField
                    label={t(
                      docker
                        ? "Devcontainer configuration"
                        : "Image or Kubernetes settings (JSON)",
                    )}
                    htmlFor={`${id}-config`}
                  >
                    {docker ? (
                      <input
                        id={`${id}-config`}
                        placeholder=".devcontainer/devcontainer.json"
                        value={config}
                        onChange={(event) => set("config", event.target.value)}
                      />
                    ) : (
                      <textarea
                        id={`${id}-config`}
                        rows={4}
                        value={config}
                        onChange={(event) => set("config", event.target.value)}
                      />
                    )}
                  </FormField>
                </>
              )}
              {error && <ErrorNotice error={error} local />}
            </Form>
          </ScrollArea>
        </DialogBody>
        <DialogFooter>
          <Button type="submit" loading={busy} disabled={busy || !valid}>
            {t("Confirm")}
          </Button>
          <Button type="button" onClick={dialog.dismiss} disabled={busy}>
            {t("Cancel")}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  );
}
