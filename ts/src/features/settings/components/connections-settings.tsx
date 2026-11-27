import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useContext, useEffect, useMemo, useState } from "react";
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
import {
  client,
  list as savedConnections,
} from "#src/shared/api/connections.ts";
import { ConnectionScope } from "#src/shared/navigation/resource-route.ts";
import { useRouteConnections } from "#src/app/router.tsx";
import { InstallationFileMenu } from "./installation-files";
import { ResourceTableSection } from "#src/shared/components/resource-layout.tsx";
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
  const scope = useContext(ConnectionScope);
  const catalog = useRouteConnections();
  // The client is made once per mount rather than per render. As a default
  // parameter it was made again on every render, and the effect that reads it
  // depends on its identity -- so answering the list re-rendered, which built
  // another client, which re-ran the effect: 3,400 lists a second until the tab
  // stopped responding. A caller that passes one in already has a stable one.
  // See cxz#230.
  const origin = new URL(c.baseUrl, location.origin).origin;
  const api = useMemo(() => {
    if (connections) return connections;
    const entries = catalog.flatMap((entry) =>
      entry.entry ? [entry.entry] : [],
    );
    if (!entries.length) return client(origin);
    if (entries.length === 1 && entries[0].transport === "origin")
      entries.push(...savedConnections(origin).slice(1));
    return { list: async () => ({ items: entries }) };
  }, [connections, catalog, origin]);
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
      <ResourceTableSection>
        {error && <ErrorNotice className={styles.error} error={error} />}
        <Table
          className={`connection-list ${resourceTableStyles.panelTable} ${resourceTableStyles.layout}`}
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
              <TableRow
                key={JSON.stringify([item.name, item.target])}
                data-read-only={item.readOnly}
              >
                <TableHead scope="row">
                  <span
                    className={resourceTableStyles.identity}
                    title={item.name}
                  >
                    <span className={resourceTableStyles.identityTitle}>
                      {item.name}
                    </span>
                    {item.isDefault && (
                      <span className={resourceTableStyles.identityDetail}>
                        {t("default")}
                      </span>
                    )}
                  </span>
                </TableHead>
                <TableCell>
                  <span
                    className={`${resourceTableStyles.identityTitle} ${styles.target}`}
                    title={item.target}
                  >
                    {item.target}
                  </span>
                </TableCell>
                <TableCell>
                  <span className={resourceTableStyles.identity}>
                    <span>{item.transport}</span>
                    {item.readOnly && (
                      <span
                        className={resourceTableStyles.identityDetail}
                        title={t("served this page")}
                      >
                        {t("served this page")}
                      </span>
                    )}
                  </span>
                </TableCell>
                <TableCell className={resourceTableStyles.rowActionCell}>
                  <InstallationFileMenu
                    c={c}
                    name={item.name || item.target}
                    active={
                      item.transport === "gateway"
                        ? `@${item.name}` === scope
                        : item.target.replace(/\/$/, "") === origin
                    }
                  />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </ResourceTableSection>
    </section>
  );
}
