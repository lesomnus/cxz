import { useId } from "react";
import {
  Button,
  Form,
  FormField,
  Popover,
  Section,
  SectionGroup,
  ValueMenu,
} from "@lesomnus/nox";
import { SourceEditor } from "@lesomnus/nox/editor";
import type { Memory } from "#gen/cxz/memory_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Project } from "#gen/cxz/project_pb";
import { EditableValue } from "#src/shared/components/editable-value.tsx";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import { ResourceFields } from "#src/shared/components/resource-fields.tsx";
import { ResourceNavigationButton } from "#src/shared/components/resource-navigation-button.tsx";
import { SessionNavigationButton } from "#src/shared/components/session-navigation-button.tsx";
import {
  projectLocation,
  resourceHandle,
} from "#src/shared/navigation/resource-route.ts";
import { Markdown } from "#src/shared/content/markdown.tsx";
import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import { publicUUID } from "#src/shared/logs/rpc-diagnostics.ts";
import { useEditorSettings } from "#src/shared/settings/settings.ts";
import { useTheme } from "#src/shared/theme/theme.tsx";
import actionStyles from "#src/shared/components/resource-action.module.css";
import styles from "./memory-document.module.css";

export type MemoryDocumentAction = {
  label: string;
  icon: IconName;
  disabled: boolean;
  danger?: boolean;
  run: () => unknown;
};

export function MemoryDocument({
  memory,
  project = memory?.project,
  name,
  content,
  loading,
  error,
  busy,
  creating,
  tags,
  actions,
  rename,
  retry,
  restore,
  purge,
  labels,
  owner,
  sessions,
  changeContent,
  changeLabels,
  changeOwner,
}: {
  memory?: Memory;
  project?: Project;
  name: string;
  content: string;
  loading: boolean;
  error: string;
  busy: boolean;
  creating: boolean;
  tags: [string, string][];
  actions: MemoryDocumentAction[];
  rename: (value: string) => Promise<void>;
  retry: () => void;
  restore: () => unknown;
  purge: () => unknown;
  labels: string;
  owner: string;
  sessions: Session[];
  changeContent: (value: string) => void;
  changeLabels: (value: string) => void;
  changeOwner: (value: string) => void;
}) {
  const id = useId();
  const theme = useTheme();
  const settings = useEditorSettings();
  const trash = !!memory?.dateDeleted;
  const ownerSession = sessions.find(
    (session) => resourceHandle({ id: session.id }) === memory?.ownerSession,
  );
  const pending = loading || creating;
  const unavailable = <span className={styles.unavailable}>—</span>;
  const title = (
    <h2 className={styles.title}>
      {(creating && !busy) || (memory && !trash) ? (
        <EditableValue
          label="Memory name"
          value={name}
          display={(value) => value || "New memory"}
          save={rename}
          validate={(value) =>
            !value.trim() ? "A memory name is required." : undefined
          }
        />
      ) : (
        <span>{name || (creating ? "New memory" : "Memory")}</span>
      )}
    </h2>
  );
  return (
    <SectionGroup
      className={styles.document}
      role="region"
      aria-label="Memory document"
      aria-busy={loading}
    >
      <Section className={styles.information} aria-label="Memory information">
        <ResourceFields
          columns={6}
          className={styles.fields}
          rows={[
            { label: "Title", value: title, columnSpan: 3 },
            {
              label: "Owner session",
              value: pending ? (
                unavailable
              ) : ownerSession ? (
                <SessionNavigationButton session={ownerSession} />
              ) : memory?.ownerSession ? (
                memory.ownerSession
              ) : (
                unavailable
              ),
              identifier: !ownerSession,
            },
            ["Revision", pending ? unavailable : memory?.revision.toString()],
            {
              label: "Memory ID",
              value: pending
                ? unavailable
                : memory && resourceHandle({ id: memory.id }),
              identifier: true,
            },
            {
              label: "Created",
              value: pending ? (
                unavailable
              ) : (
                <DateTime value={memory?.dateCreated} showOffset={false} />
              ),
              labelDetail: !pending && memory?.dateCreated && (
                <RelativeTime value={memory.dateCreated} />
              ),
              columnSpan: 2,
            },
            {
              label: "Updated",
              value: pending ? (
                unavailable
              ) : (
                <DateTime value={memory?.dateUpdated} showOffset={false} />
              ),
              labelDetail: !pending && memory?.dateUpdated && (
                <RelativeTime value={memory.dateUpdated} />
              ),
              columnSpan: 2,
            },
            {
              label: "Project",
              value: project ? (
                <ResourceNavigationButton
                  {...projectLocation(resourceHandle(project))}
                  name={
                    project.name || project.alias || resourceHandle(project)
                  }
                  alias={project.alias}
                />
              ) : (
                unavailable
              ),
              columnSpan: 2,
            },
            {
              label: "Labels",
              value:
                !pending && tags.length ? (
                  <div className={styles.tags}>
                    {tags.map(([key, value]) => (
                      <span key={key}>
                        {key}={value}
                      </span>
                    ))}
                  </div>
                ) : (
                  unavailable
                ),
              columnSpan: 4,
            },
          ]}
        />
      </Section>
      <SectionGroup
        className={styles.actions}
        role="group"
        aria-label="Memory document actions"
      >
        {actions.map((action) => (
          <Popover key={action.label} content={action.label} placement="top">
            <Section
              as="button"
              type="button"
              className={`${actionStyles.button} ${action.danger ? actionStyles.danger : ""}`}
              aria-label={action.label}
              disabled={action.disabled}
              onClick={action.run}
            >
              <Icon name={action.icon} />
            </Section>
          </Popover>
        ))}
      </SectionGroup>
      <Section className={styles.bodySection} aria-label="Memory content">
        {loading ? (
          <p className={styles.empty}>Loading memory…</p>
        ) : error && !memory && !creating ? (
          <Button onClick={retry}>Retry memory</Button>
        ) : (
          <>
            {trash && (
              <div className={styles.trashNotice}>
                <p>
                  In trash until{" "}
                  {memory?.dateDeleted
                    ? new Date(
                        Number(memory.dateDeleted.seconds) * 1000,
                      ).toLocaleString()
                    : ""}
                </p>
                <div className={styles.trashActions}>
                  <Button disabled={busy || !memory} onClick={restore}>
                    Restore
                  </Button>
                  <Button disabled={busy || !memory} onClick={purge}>
                    Purge
                  </Button>
                </div>
              </div>
            )}
            {creating ? (
              <>
                <Form as="fieldset" columns={1} disabled={busy}>
                  <FormField label="Labels" htmlFor={id + "-labels"}>
                    <input
                      id={id + "-labels"}
                      aria-label="Memory labels"
                      placeholder="topic=design"
                      value={labels}
                      onChange={(event) => changeLabels(event.target.value)}
                    />
                  </FormField>
                  {sessions.length > 0 && (
                    <FormField label="Owner session">
                      <ValueMenu
                        label="Owner session"
                        value={owner}
                        options={[
                          { value: "", label: "No owner session" },
                          ...sessions.map((session) => ({
                            value:
                              publicUUID({
                                key: { case: "id", value: session.id },
                              }) ?? "",
                            label:
                              session.alias ||
                              session.name ||
                              session.runtimeId,
                          })),
                        ]}
                        choose={changeOwner}
                        disabled={busy}
                      />
                    </FormField>
                  )}
                </Form>
                <div className={styles.editor}>
                  <SourceEditor
                    path="memory-new.md"
                    ariaLabel="Memory content"
                    value={content}
                    readOnly={busy}
                    onChange={changeContent}
                    theme={theme}
                    settings={settings}
                  />
                </div>
              </>
            ) : (
              <div className={styles.preview}>
                {content.trim() ? (
                  <Markdown text={content} />
                ) : (
                  <p className={styles.empty}>This document is empty.</p>
                )}
              </div>
            )}
          </>
        )}
      </Section>
    </SectionGroup>
  );
}
