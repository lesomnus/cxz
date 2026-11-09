import {
  Button,
  CompactNumber,
  Popover,
  Section,
  SectionGroup,
  ValueMenu,
} from "@lesomnus/nox";
import type { Memory } from "#gen/cxz/memory_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Project } from "#gen/cxz/project_pb";
import type { MemoryVersion } from "#gen/cxz/memory_svc_pb";
import { EditableValue } from "#src/shared/components/editable-value.tsx";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import {
  dateTimeDate,
  formatLocalDateTime,
} from "#src/shared/components/date-time-format.ts";
import { MemoryVersionChanges } from "./memory-version-changes";
import { ResourceFields } from "#src/shared/components/resource-fields.tsx";
import { ResourceNavigationButton } from "#src/shared/components/resource-navigation-button.tsx";
import { SessionNavigationButton } from "#src/shared/components/session-navigation-button.tsx";
import {
  projectLocation,
  resourceHandle,
} from "#src/shared/navigation/resource-route.ts";
import {
  MemoryPreview,
  type MemoryComparison,
  type MemoryPreviewMode,
} from "./memory-preview";
import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import actionStyles from "#src/shared/components/resource-action.module.css";
import styles from "./memory-document.module.css";

export type MemoryDocumentAction = {
  label: string;
  feedback?: string;
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
  revision,
  versions,
  versionsLoading,
  versionsError,
  changeRevision,
  previewMode,
  changePreviewMode,
  comparison,
  actions,
  rename,
  retry,
  restore,
  purge,
  sessions,
}: {
  memory?: Memory;
  project?: Project;
  name: string;
  content: string;
  loading: boolean;
  error: string;
  busy: boolean;
  revision: bigint;
  versions: MemoryVersion[];
  versionsLoading: boolean;
  versionsError: string;
  changeRevision: (revision: bigint) => void;
  previewMode: MemoryPreviewMode;
  changePreviewMode: (mode: MemoryPreviewMode) => void;
  comparison: MemoryComparison;
  actions: MemoryDocumentAction[];
  rename: (value: string) => Promise<void>;
  retry: () => void;
  restore: () => unknown;
  purge: () => unknown;
  sessions: Session[];
}) {
  const trash = !!memory?.dateDeleted;
  const ownerSession = sessions.find(
    (session) => resourceHandle({ id: session.id }) === memory?.ownerSession,
  );
  const pending = loading && !memory;
  const revisions = new Map(
    versions.map((version) => [version.revision, version]),
  );
  const revisionOptions = memory
    ? [
        { value: "0", label: `Revision r${memory.revision} · Latest` },
        ...versions
          .filter((version) => version.revision !== memory.revision)
          .map((version) => ({
            value: version.revision.toString(),
            label: `Revision r${version.revision}`,
          })),
      ].map((option) => {
        const number =
          option.value === "0" ? memory.revision : BigInt(option.value);
        const version = revisions.get(number);
        const created =
          version?.dateCreated ??
          (number === 1n ? memory.dateCreated : undefined);
        const date = dateTimeDate(created);
        return {
          ...option,
          description: date ? formatLocalDateTime(date) : "—",
        };
      })
    : [];
  const unavailable = <span className={styles.unavailable}>—</span>;
  const title = (
    <h2 className={styles.title}>
      {memory && !trash ? (
        <EditableValue
          label="Memory name"
          value={name}
          display={(value) => value || "Memory"}
          save={rename}
          validate={(value) =>
            !value.trim() ? "A memory name is required." : undefined
          }
        />
      ) : (
        <span>{name || "Memory"}</span>
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
          columns={4}
          className={styles.fields}
          rows={[
            { label: "Title", value: title, fullWidth: true },
            {
              label: "Project",
              hideLabel: true,
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
            },
            {
              label: "Session",
              hideLabel: true,
              value: pending ? (
                unavailable
              ) : ownerSession ? (
                <SessionNavigationButton session={ownerSession} />
              ) : (
                memory?.ownerSession || unavailable
              ),
              identifier: !ownerSession,
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
            },
          ]}
        />
      </Section>
      <SectionGroup className={styles.side}>
        <SectionGroup
          className={styles.actions}
          role="group"
          aria-label="Memory document actions"
        >
          {actions.map((action) => (
            <Popover
              key={action.label}
              content={action.feedback ?? action.label}
              placement="top"
            >
              <Section
                as="button"
                type="button"
                className={`${actionStyles.button} ${action.danger ? actionStyles.danger : ""}`}
                aria-label={action.feedback ?? action.label}
                disabled={action.disabled}
                onClick={action.run}
              >
                <Icon name={action.icon} />
              </Section>
            </Popover>
          ))}
        </SectionGroup>
        <Section className={styles.revision} aria-label="Memory revision">
          <ValueMenu
            label="Revision"
            value={revision.toString()}
            display={
              memory ? `Revision r${revision || memory.revision}` : "Revision —"
            }
            options={revisionOptions}
            renderLabel={(option) => {
              const number =
                option.value === "0" ? memory?.revision : BigInt(option.value);
              const version =
                number === undefined ? undefined : revisions.get(number);
              const created =
                version?.dateCreated ??
                (number === 1n ? memory?.dateCreated : undefined);
              return (
                <span className={styles.revisionValue}>
                  <span className={styles.revisionRow}>
                    <span className={styles.revisionNumber}>
                      r
                      {number !== undefined ? (
                        <CompactNumber
                          value={Number(number)}
                          mode="integer"
                          digits={3}
                          label={`Revision ${number}`}
                        />
                      ) : (
                        "—"
                      )}
                    </span>
                    <MemoryVersionChanges
                      version={version}
                      className={styles.revisionChanges}
                    />
                  </span>
                  <span className={styles.revisionRow}>
                    <span className={styles.revisionDate}>
                      <DateTime value={created} showOffset={false} />
                    </span>
                    <span className={styles.revisionAge}>
                      {created ? <RelativeTime value={created} /> : "—"}
                    </span>
                  </span>
                </span>
              );
            }}
            disabled={
              busy ||
              loading ||
              pending ||
              !memory ||
              versionsLoading ||
              !!versionsError
            }
            disabledReason={versionsError || undefined}
            choose={(value) => changeRevision(BigInt(value))}
          />
        </Section>
      </SectionGroup>
      <Section className={styles.bodySection} aria-label="Memory content">
        {loading ? (
          <p className={styles.empty}>Loading memory…</p>
        ) : error ? (
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
            <MemoryPreview
              content={content}
              mode={previewMode}
              changeMode={changePreviewMode}
              comparison={comparison}
            />
          </>
        )}
      </Section>
    </SectionGroup>
  );
}
