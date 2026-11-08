import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useEffect, useMemo, useState } from "react";
import { t } from "#src/shared/i18n/i18n.ts";
import type { Connection } from "#gen/cxz/connection_pb";
import { Section, TableScrollArea } from "@lesomnus/nox";
import { ResourcePage } from "#src/shared/components/resource-layout.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import {
  SettingsContent,
  SettingsToolbar,
} from "#src/features/settings/components/settings-content.tsx";
import pageStyles from "#src/shared/components/resource-page.module.css";
import { client } from "#src/shared/api/connections.ts";

// What this browser is attached to. One connection reaches an installation
// whole -- every project and session on it -- so this answers "whose projects
// am I looking at", which until now nothing did.
//
// Read-only for the moment, and for a direct connection that is the whole
// truth: the origin the page came from cannot be edited or forgotten, and it
// may well be the only entry. See docs/plans/web-connections.md.
export function ConnectionsPage({
  connections,
}: {
  connections?: { list: (request: object) => Promise<{ items: Connection[] }> };
}) {
  const [items, setItems] = useState<Connection[]>([]);
  const [error, setError] = useState("");
  // The client is made once per mount rather than per render. As a default
  // parameter it was made again on every render, and the effect that reads it
  // depends on its identity -- so answering the list re-rendered, which built
  // another client, which re-ran the effect: 3,400 lists a second until the tab
  // stopped responding. A caller that passes one in already has a stable one.
  // See cxz#230.
  const api = useMemo(() => connections ?? client(), [connections]);
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
    <ResourcePage className="settings-page">
      <SettingsToolbar aria-label={t("Settings")} />

      <PanelScroll
        className="settings-form-pane"
        contentClassName={pageStyles.contentPadding}
        aria-label={t("Connections")}
      >
        <SettingsContent>
          <div className="settings-editor-body">
            <header>
              <div>
                <h1>{t("Connections")}</h1>
                <small>
                  {t(
                    "A connection reaches one installation and every project on it.",
                  )}
                </small>
              </div>
            </header>
            {error ? <ErrorNotice error={error} /> : null}
            <div className="settings-editor-groups">
              <Section className="settings-group">
                <TableScrollArea
                  scrollbarLabel={t("Connections")}
                  style={{ height: "auto" }}
                >
                  <table className="connection-list">
                    <thead>
                      <tr>
                        <th scope="col">{t("Name")}</th>
                        <th scope="col">{t("Target")}</th>
                        <th scope="col">{t("Transport")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {items.map((c) => (
                        <tr key={c.target} data-read-only={c.readOnly}>
                          <th scope="row">
                            {c.name}
                            {c.isDefault ? ` · ${t("default")}` : ""}
                          </th>
                          <td>{c.target}</td>
                          <td>
                            {c.transport}
                            {c.readOnly ? ` · ${t("served this page")}` : ""}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </TableScrollArea>
              </Section>
            </div>
          </div>
        </SettingsContent>
      </PanelScroll>
    </ResourcePage>
  );
}
