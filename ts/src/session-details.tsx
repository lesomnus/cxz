import { useState, type ReactNode } from "react";
import type { Session } from "../gen/cxz/session_pb";
import { EditableValue } from "./editable-value";
import { validateSessionField, type SessionField } from "./session-edit";
import { SessionIdentity } from "./session-identity";
import { formatReset, formatTokens, type SessionInfo } from "./session-info";
import { t, currentLocale } from "./i18n";
import { useLocale } from "./i18n-react";

export function SessionDetails({
  session,
  info,
  edit,
}: {
  session: Session;
  info: SessionInfo;
  edit: (field: SessionField, value: string) => Promise<Session>;
}) {
  useLocale();
  const [value, setValue] = useState(session);
  async function save(field: SessionField, text: string) {
    const updated = await edit(field, text);
    setValue(updated);
  }
  const date = (value: Session["dateCreated"]) =>
    value
      ? new Date(
          Number(value.seconds) * 1000 + value.nanos / 1e6,
        ).toLocaleString(currentLocale())
      : "—";
  const rows: [string, ReactNode][] = [
    [t("State"), value.status?.state],
    [
      t("Title"),
      <EditableValue
        label={t("Title")}
        value={value.name}
        save={(text) => save("name", text)}
        validate={(text) => validateSessionField("name", text)}
      />,
    ],
    [
      t("Alias"),
      <EditableValue
        label={t("Alias")}
        value={value.alias}
        save={(text) => save("alias", text)}
        validate={(text) => validateSessionField("alias", text)}
        monospace
      />,
    ],
    [t("Project"), value.project?.name || value.project?.alias],
    [t("Workspace"), value.project?.workspace],
    [t("Agent"), value.agent],
    [t("Model"), info.model],
    [t("Effort"), info.effort],
    [t("Runtime ID"), value.runtimeId],
    [t("Run ID"), value.status?.runId],
    [t("Provider session ID"), value.status?.vendorId],
    [t("Permission mode"), value.status?.permissionMode],
    [t("Last event"), value.status?.lastSeq?.toString()],
    [t("Pending approvals"), value.status?.pending.length ?? 0],
    [
      t("Remaining usage"),
      info.remaining === undefined ? undefined : `${info.remaining}%`,
    ],
    [t("Usage reset"), formatReset(info.reset)],
    [
      t("Context"),
      `${formatTokens(info.contextUsed)} / ${formatTokens(info.contextWindow)}`,
    ],
    [t("Created"), date(value.dateCreated)],
    [t("Updated"), date(value.dateUpdated)],
  ];
  return (
    <>
      <SessionIdentity session={value} heading />
      <dl className="session-details">
        {rows.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{value || value === 0 ? value : "—"}</dd>
          </div>
        ))}
      </dl>
    </>
  );
}
