import { projectId } from "#src/shared/api/project-id.ts";
import {
  useLocalError,
  useLocalErrors,
} from "#src/shared/errors/error-context.tsx";
import { useRef, useState } from "react";
import {
  ActionMenu,
  ActionError,
  Popover,
  Section,
  SectionGroup,
} from "@lesomnus/nox";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { SessionActionIcon } from "#src/features/session/components/session-action-icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { eraseProject } from "./project-erase";
import { trustRequired } from "./config-trust";
import {
  removeContainer,
  startContainer,
  stopContainer,
} from "./project-container";
import styles from "./project-actions.module.css";
import actionStyles from "#src/shared/components/resource-action.module.css";
import {
  canControlProject,
  projectOperations,
  settleProjects,
  type ProjectOperation,
} from "./project-control-batch";

type Action = ProjectOperation;

// What a button says is the name of the call it makes. Stopping keeps the
// container, starting brings it back, removing throws the container away and
// keeps everything else, and erasing is the one that deletes data.
// See docs/plans/project-lifecycle-api.md.
// Each action's glyph. Erasing borrows the trash: it is the one that deletes.
const icons: Record<Action, "stop" | "resume" | "remove" | "erase"> = {
  stop: "stop",
  start: "resume",
  remove: "remove",
  erase: "erase",
};

const copy: Record<
  Action,
  {
    label: string;
    title: string;
    confirm: string;
    body: string;
    note: string;
  }
> = {
  stop: {
    label: t("Stop"),
    title: t("Stop project"),
    confirm: t("Stop"),
    body: t(
      "Stops the project container and all its sessions. The container is kept, so starting it again takes seconds.",
    ),
    note: t("The project remains listed and can be started again."),
  },
  start: {
    label: t("Start"),
    title: t("Start project"),
    confirm: t("Start"),
    body: t(
      "Makes the project run: the container it already has, or a new one built from its configuration, which takes minutes. Agents stay stopped until a session is resumed.",
    ),
    note: t("Workspace source files and shared accounts are preserved."),
  },
  remove: {
    label: t("Remove"),
    title: t("Remove container"),
    confirm: t("Remove"),
    body: t(
      "Removes the project container. The project, its sessions, conversation history and volumes are retained; starting again builds a new container.",
    ),
    note: t("The project remains listed and can be started again."),
  },
  erase: {
    label: t("Erase"),
    title: t("Erase project"),
    confirm: t("Erase permanently"),
    body: t(
      "Permanently deletes this project, its sessions, conversation history, shared memory and owned resources. This cannot be undone.",
    ),
    note: t("Workspace source files and shared accounts are preserved."),
  },
};
export function ProjectActions({
  c,
  project,
  projects = [],
  sessions,
  erased,
  updated,
  variant = "buttons",
  completed,
  refresh,
  refreshing = false,
}: {
  c: Connection;
  project?: Project;
  projects?: Project[];
  sessions: Session[];
  erased: (project: Project) => void;
  updated: (project: Project) => void;
  variant?: "buttons" | "menu";
  completed?: (projects: Project[]) => void;
  refresh?: () => Promise<void>;
  refreshing?: boolean;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const refreshTrigger = useRef<HTMLButtonElement>(null);
  const [refreshError, setRefreshError] = useState("");
  const localRefreshError = useLocalError(refreshError);
  type Failure = { project: Project; error: string };
  const [target, setTarget] = useState<{
    kind: Action;
    projects: Project[];
    failures: Failure[];
  }>();
  // A start the configuration refused until someone says yes to it: the
  // findings are what they are saying yes to.
  const [trust, setTrust] = useState<
    {
      project: Project;
      findings: string[];
    }[]
  >([]);
  const [trustFailures, setTrustFailures] = useState<Failure[]>([]);
  const visibleFailures = useLocalErrors(target?.failures ?? []);
  const visibleTrustFailures = useLocalErrors(trustFailures);
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const advanceTrust = useRef(false);
  const bulk = !project;
  const scope = project ? [project] : projects;
  const order: readonly Action[] =
    variant === "menu"
      ? projectOperations
      : ["erase", "remove", "stop", "start"];
  const actions = order.map((kind) => ({
    kind,
    disabled: busy || !scope.some((p) => canControlProject(p, kind)),
  }));
  function request(kind: Action, button: HTMLButtonElement | null) {
    if (lock.current) return;
    const projects = scope.filter((p) => canControlProject(p, kind));
    if (!projects.length) return;
    trigger.current = button;
    advanceTrust.current = false;
    setTrust([]);
    setTrustFailures([]);
    setTarget({ kind, projects, failures: [] });
  }
  async function execute(kind: Action, projects: Project[], trusted = false) {
    if (lock.current) return false;
    lock.current = true;
    setBusy(true);
    const acting = bulk
      ? {
          projects: c.projects,
          store: c.store,
          drafts: c.drafts,
          attention: c.attention,
          conversations: c.conversations,
          inventory: { snapshot: c.inventory.snapshot, refresh: () => {} },
        }
      : c;
    try {
      const results = await settleProjects(projects, async (p) => {
        if (kind === "erase") {
          await eraseProject(acting, p, sessions);
          erased(p);
        } else {
          const result =
            kind === "start"
              ? await startContainer(acting, p, trusted)
              : kind === "stop"
                ? await stopContainer(acting, p)
                : await removeContainer(acting, p);
          updated(result);
        }
      });
      completed?.(results.filter((r) => r.ok).map((r) => r.project));
      const failures: Failure[] = [];
      const awaiting: typeof trust = [];
      for (const result of results) {
        if (result.ok) continue;
        const findings =
          kind === "start" && !trusted
            ? trustRequired(result.error)
            : undefined;
        if (findings) awaiting.push({ project: result.project, findings });
        else
          failures.push({
            project: result.project,
            error: String(result.error),
          });
      }
      if (trusted) {
        setTrustFailures(failures);
        if (failures.length) {
          const ids = new Set(failures.map((f) => projectId(f.project)));
          setTrust((current) =>
            current.filter((v) => ids.has(projectId(v.project))),
          );
        }
      } else {
        if (awaiting.length) setTrust((current) => [...current, ...awaiting]);
        advanceTrust.current =
          !failures.length && (awaiting.length > 0 || trust.length > 0);
        if (failures.length)
          setTarget({
            kind,
            projects: failures.map((f) => f.project),
            failures,
          });
      }
      return !failures.length;
    } finally {
      if (bulk) c.inventory.refresh();
      lock.current = false;
      setBusy(false);
    }
  }
  const titles = {
    stop: t("Stop projects"),
    start: t("Start projects"),
    remove: t("Remove containers"),
    erase: t("Erase projects"),
  };
  const bodies = {
    stop: t(
      "Stops the selected project containers and all their sessions. Their data is retained.",
    ),
    start: t(
      "Starts the selected project containers, creating missing containers from their configuration. Agents remain stopped.",
    ),
    remove: t(
      "Removes the selected project containers. Projects, sessions, history and volumes are retained.",
    ),
    erase: t(
      "Permanently deletes the selected projects and their owned data. This cannot be undone.",
    ),
  };
  const failedMessage = (failures: Failure[]) =>
    failures.length === 1
      ? failures[0]!.error
      : failures.length
        ? t(
            "Some project actions failed. Successful projects will not be retried.",
          )
        : t("Action failed. Please try again.");
  return (
    <>
      {variant === "menu" ? (
        <div ref={menu} className={styles.menu}>
          <ActionMenu
            label={
              bulk
                ? t("Selected project actions")
                : t("Controls for {name}", { name: label(project!) })
            }
            status={
              bulk
                ? t("{count} selected", { count: projects.length })
                : undefined
            }
            disabled={busy || !scope.length}
            triggerClassName={
              bulk ? styles.headerMenuTrigger : styles.menuTrigger
            }
            motion={{ from: "top-right", effect: "reveal" }}
            items={actions.map((action) => ({
              label: copy[action.kind].label,
              icon: <SessionActionIcon action={icons[action.kind]} />,
              disabled: action.disabled,
              danger: action.kind === "erase",
              run: () => {
                request(
                  action.kind,
                  menu.current?.querySelector("button") ?? null,
                );
              },
            }))}
          />
        </div>
      ) : (
        <SectionGroup className={styles.buttons}>
          {actions.map((action) => (
            <Popover
              key={action.kind}
              content={copy[action.kind].label}
              placement="top"
            >
              <Section
                as="button"
                type="button"
                effect="sparkle"
                className={`${actionStyles.button} ${styles.action} ${action.kind === "erase" ? actionStyles.danger : ""}`}
                aria-label={copy[action.kind].label}
                disabled={action.disabled}
                onClick={(event) => {
                  request(action.kind, event.currentTarget);
                }}
              >
                <SessionActionIcon action={icons[action.kind]} />
              </Section>
            </Popover>
          ))}
          {refresh && (
            <Popover content={t("Refresh")} placement="top">
              <Section
                as="button"
                ref={refreshTrigger}
                className={`${actionStyles.button} ${styles.action}`}
                aria-label={t("Refresh")}
                loading={refreshing || busy}
                onClick={async () => {
                  setRefreshError("");
                  try {
                    await refresh();
                  } catch (error) {
                    setRefreshError(String(error));
                  }
                }}
              >
                <SessionActionIcon action="restart" />
              </Section>
            </Popover>
          )}
        </SectionGroup>
      )}
      {localRefreshError && (
        <ActionError
          anchor={refreshTrigger.current}
          onDismiss={() => setRefreshError("")}
        >
          {localRefreshError}
        </ActionError>
      )}
      {target && (
        <ConfirmationDialog
          title={
            target.projects.length > 1
              ? titles[target.kind]
              : copy[target.kind].title
          }
          confirmLabel={copy[target.kind].confirm}
          close={() => {
            setTarget(undefined);
            if (!advanceTrust.current) setTrust([]);
            advanceTrust.current = false;
            trigger.current?.focus({ preventScroll: true });
          }}
          failureMessage={failedMessage(target.failures)}
          execute={() => execute(target.kind, target.projects)}
        >
          {target.projects.length === 1 ? (
            <p>
              <strong>{label(target.projects[0]!)}</strong>
            </p>
          ) : (
            <ul className={styles.targets}>
              {target.projects.map((p) => (
                <li key={projectId(p)}>
                  <strong>{label(p)}</strong>
                </li>
              ))}
            </ul>
          )}
          <p>
            {target.projects.length > 1
              ? bodies[target.kind]
              : copy[target.kind].body}
          </p>
          <p className="muted">{copy[target.kind].note}</p>
          {visibleFailures.length > 1 && (
            <ul className={styles.failures}>
              {visibleFailures.map((f) => (
                <li key={projectId(f.project)}>
                  {label(f.project)}: {f.error}
                </li>
              ))}
            </ul>
          )}
        </ConfirmationDialog>
      )}
      {!target && trust.length > 0 && (
        <ConfirmationDialog
          title={t("Trust configuration")}
          confirmLabel={t("Trust and start")}
          close={() => {
            setTrust([]);
            setTrustFailures([]);
            trigger.current?.focus({ preventScroll: true });
          }}
          failureMessage={failedMessage(trustFailures)}
          execute={() =>
            execute(
              "start",
              trust.map((v) => v.project),
              true,
            )
          }
        >
          <p>
            <strong>{trust.map((v) => label(v.project)).join(", ")}</strong>
          </p>
          <p>
            {t(
              "This project's devcontainer configuration asks for access beyond its container. Start it only if you trust this configuration.",
            )}
          </p>
          <ul className={styles.findings}>
            {trust.flatMap((v) =>
              v.findings.map((finding) => (
                <li key={`${projectId(v.project)}/${finding}`}>
                  {trust.length > 1 && <strong>{label(v.project)}: </strong>}
                  <code>{finding}</code>
                </li>
              )),
            )}
          </ul>
          <p className="muted">
            {t("You will be asked again if the configuration changes.")}
          </p>
          {visibleTrustFailures.length > 1 && (
            <ul className={styles.failures}>
              {visibleTrustFailures.map((f) => (
                <li key={projectId(f.project)}>
                  {label(f.project)}: {f.error}
                </li>
              ))}
            </ul>
          )}
        </ConfirmationDialog>
      )}
    </>
  );
}

function label(project: Project) {
  return project.name || project.alias || projectId(project);
}
