import { createClient, type Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { Store } from "@lesomnus/payday/store";
import { Queries } from "@lesomnus/payday/query";
import { entities } from "#gen/entities";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import { SessionService } from "#gen/cxz/session_svc_pb";
import type { ComposerPaste } from "#src/features/session/composer/composer-pastes.ts";
import type { EditorState } from "#src/features/workspace/editor/workspace-editor.tsx";
import { ResourceInventory } from "./resource-inventory";
import { SessionDrafts } from "#src/features/session/model/session-drafts.ts";
import { SessionAttention } from "#src/features/session/model/session-attention.ts";

// All client state belongs to one authenticated Connection. No credentials or
// conversation cache are persisted in localStorage. Drafts use tab-local storage.
export class Connection {
  readonly clientId = crypto.randomUUID();
  readonly transport;
  readonly store;
  readonly queries;
  readonly projects;
  readonly sessions;
  readonly inventory;
  readonly attention;
  // Original paste bodies stay in this authenticated connection's memory, like drafts.
  readonly pastes = new Map<string, ComposerPaste>();
  readonly drafts;
  readonly editors = new Map<string, EditorState>();
  readonly inProcess: boolean;
  constructor(
    readonly baseUrl = location.origin,
    transport?: Transport,
  ) {
    this.inProcess = !!transport;
    this.drafts = new SessionDrafts(baseUrl, this.pastes);
    this.transport =
      transport ??
      createConnectTransport({
        baseUrl,
        fetch: (input, init) =>
          fetch(input, { ...init, credentials: "same-origin" }),
      });
    this.store = Store.open(entities, {
      name: "cxz",
      identity: crypto.randomUUID(),
    });
    this.queries = new Queries(this.store, this.transport, entities);
    this.projects = createClient(ProjectService, this.transport);
    this.sessions = createClient(SessionService, this.transport);
    this.attention = new SessionAttention(this.sessions, baseUrl);
    this.inventory = new ResourceInventory(
      this.projects,
      this.sessions,
      this.store,
    );
  }
}
export const ref = (id: string) => ({
  key: { case: "runtimeId" as const, value: id },
});
export async function authenticate(token: string) {
  const r = await fetch("/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ token }),
  });
  if (!r.ok) throw new Error(await r.text());
}
