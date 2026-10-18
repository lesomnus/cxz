import { useEffect, useState } from "react";
import { t } from "#src/shared/i18n/i18n.ts";
import type { Connection } from "#gen/cxz/connection_pb";
import { client } from "#src/shared/api/connections.ts";

// What this browser is attached to. One connection reaches an installation
// whole -- every project and session on it -- so this answers "whose projects
// am I looking at", which until now nothing did.
//
// Read-only for the moment, and for a direct connection that is the whole
// truth: the origin the page came from cannot be edited or forgotten, and it
// may well be the only entry. See docs/plans/web-connections.md.
export function ConnectionsPage({
  connections = client(),
}: {
  connections?: { list: (request: object) => Promise<{ items: Connection[] }> };
}) {
  const [items, setItems] = useState<Connection[]>([]);
  const [error, setError] = useState("");
  useEffect(() => {
    let live = true;
    connections
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
  }, [connections]);
  return (
    <main className="settings-page">
      <div className="settings-split">
        <section className="settings-form-pane" aria-label={t("Connections")}>
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
            {error ? <p role="alert">{error}</p> : null}
            <div className="settings-editor-groups">
              <section className="settings-group">
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
              </section>
            </div>
          </div>
        </section>
      </div>
    </main>
  );
}
