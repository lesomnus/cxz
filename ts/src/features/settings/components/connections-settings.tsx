import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useEffect, useMemo, useState } from "react";
import { t } from "#src/shared/i18n/i18n.ts";
import type { Connection as ConnectionEntry } from "#gen/cxz/connection_pb";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@lesomnus/nox";
import type { Connection } from "#src/shared/api/connection.ts";
import { client } from "#src/shared/api/connections.ts";
import { InstallationFileMenu } from "./installation-files";
import resourceTableStyles from "#src/shared/components/resource-table.module.css";
import styles from "./connections-settings.module.css";

export type SettingsConnections = {
  list: (request: object) => Promise<{ items: ConnectionEntry[] }>;
};

export function ConnectionsSettings({
  c,
  connections,
}: {
  c: Connection;
  connections?: SettingsConnections;
}) {
  const [items, setItems] = useState<ConnectionEntry[]>([]);
  const [error, setError] = useState("");
  // The client is made once per mount rather than per render. As a default
  // parameter it was made again on every render, and the effect that reads it
  // depends on its identity -- so answering the list re-rendered, which built
  // another client, which re-ran the effect: 3,400 lists a second until the tab
  // stopped responding. A caller that passes one in already has a stable one.
  // See cxz#230.
  const origin = c.baseUrl.replace(/\/$/, "");
  const api = useMemo(
    () => connections ?? client(origin),
    [connections, origin],
  );
  useEffect(() => {
    let live = true;
    api
      .list({})
      .then((reply) => {
        if (live) setItems(reply.items);
      })
      .catch((e: unknown) => {
        if (live) setError(String(e));
      });
    return () => {
      live = false;
    };
  }, [api]);
  return (
    <section
      id="settings-connections"
      className="settings-editor-body"
      data-settings-topic="connections"
      aria-labelledby="settings-connections-title"
    >
      <header>
        <div>
          <h1 id="settings-connections-title">{t("Connections")}</h1>
          <small>
            {t(
              "A connection reaches one installation and every project on it.",
            )}
          </small>
        </div>
      </header>
      {error ? <ErrorNotice error={error} /> : null}
      <Table
        className={`connection-list ${resourceTableStyles.table} ${resourceTableStyles.layout} ${styles.table}`}
        aria-label={t("Connections")}
        stickyHeader={false}
      >
        <colgroup>
          <col className={styles.nameColumn} />
          <col />
          <col className={styles.transportColumn} />
          <col className={styles.actionsColumn} />
        </colgroup>
        <TableHeader>
          <TableRow>
            <TableHead>{t("Name")}</TableHead>
            <TableHead>{t("Target")}</TableHead>
            <TableHead>{t("Transport")}</TableHead>
            <TableHead aria-label={t("Actions")} />
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((item) => (
            <TableRow key={item.target} data-read-only={item.readOnly}>
              <TableHead scope="row">
                {item.name}
                {item.isDefault && <small>{t("default")}</small>}
              </TableHead>
              <TableCell>{item.target}</TableCell>
              <TableCell>
                {item.transport}
                {item.readOnly && <small>{t("served this page")}</small>}
              </TableCell>
              <TableCell className={styles.actionsCell}>
                <InstallationFileMenu
                  c={c}
                  name={item.name || item.target}
                  active={item.target.replace(/\/$/, "") === origin}
                  triggerClassName={styles.trigger}
                />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </section>
  );
}
