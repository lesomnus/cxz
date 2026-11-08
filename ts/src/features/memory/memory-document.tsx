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
import { EditableValue } from "#src/shared/components/editable-value.tsx";
import { RelativeTime } from "#src/shared/components/date-time.tsx";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
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
  name,
  content,
  loading,
  error,
  busy,
  creating,
  ownerInfo,
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
  name: string;
  content: string;
  loading: boolean;
  error: string;
  busy: boolean;
  creating: boolean;
  ownerInfo?: { name: string; agent: string };
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
  return (
    <SectionGroup
      className={styles.document}
      role="region"
      aria-label="Memory document"
      aria-busy={loading}
    >
      <Section className={styles.titleSection} aria-label="Memory title">
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
        {!creating && memory && (
          <div className={styles.metadata}>
            <span className={styles.owner}>
              {ownerInfo?.agent && (
                <span aria-hidden="true">
                  <AgentBrand agent={ownerInfo.agent} />
                </span>
              )}
              <span>{ownerInfo?.name}</span>
            </span>
            <span>Revision {memory.revision.toString()}</span>
            {memory.dateUpdated && <RelativeTime value={memory.dateUpdated} />}
          </div>
        )}
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
            {!creating && tags.length > 0 && (
              <div className={styles.tags}>
                {tags.map(([key, value]) => (
                  <span key={key}>
                    {key}={value}
                  </span>
                ))}
              </div>
            )}
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
