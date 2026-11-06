import { PrefixValueMenu } from "@lesomnus/nox";
import type { Session } from "#gen/cxz/session_pb";
import { projectId } from "#src/shared/api/project-id.ts";
import { sessionUUID } from "#src/shared/api/session-id.ts";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import styles from "./project-picker.module.css";

const identify = (session: Session) => sessionUUID(session.id);

export function SessionPicker({
  sessions,
  project,
  value = "",
  loading = false,
  suffix = "",
  getValue = identify,
  choose,
}: {
  sessions: readonly Session[];
  project: string;
  value?: string;
  loading?: boolean;
  suffix?: string;
  getValue?: (session: Session) => string;
  choose: (value: string) => void;
}) {
  const scoped = sessions.filter(
    (session) => session.project && projectId(session.project) === project,
  );
  const selected = value
    ? scoped.find((session) => getValue(session) === value)
    : undefined;
  const display =
    selected?.name ||
    selected?.alias ||
    (value ? `Session ${value.slice(0, 8)}` : "All sessions");
  return (
    <PrefixValueMenu
      className={styles.picker}
      prefix={<ResourceIcon kind="sessions" />}
      label="Select session"
      value={value}
      display={loading ? "Loading sessions…" : display + suffix}
      options={[
        { value: "", label: "All sessions" },
        ...scoped.map((session) => ({
          value: getValue(session),
          label: session.name || session.alias || getValue(session),
          searchText: `${session.alias} ${sessionUUID(session.id)} ${session.runtimeId}`,
        })),
      ]}
      searchable
      disabled={!project || loading}
      choose={choose}
    />
  );
}
