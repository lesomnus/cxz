import { createClient, type Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { Store } from "@lesomnus/payday/store";
import { Queries } from "@lesomnus/payday/query";
import { entities } from "#gen/entities";
import { InstallationService } from "#gen/cxz/installation_pb";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import { SessionService } from "#gen/cxz/session_svc_pb";
import { AccountService } from "#gen/cxz/account_svc_pb";
import { AuthBindingService } from "#gen/cxz/auth_binding_svc_pb";
import { TaskService } from "#gen/cxz/task_svc_pb";
import { TaskCatalog } from "#src/features/tasks/task-catalog.ts";
import type { ComposerPaste } from "#src/features/session/composer/composer-pastes.ts";
import type { Transfer } from "#src/features/session/composer/composer-upload.ts";
import type { EditorState } from "#src/features/workspace/editor/workspace-editor.tsx";
import { ResourceInventory } from "./resource-inventory";
import { SessionDrafts } from "#src/features/session/model/session-drafts.ts";
import { ConversationCache } from "#src/features/session/model/conversation-cache.ts";
import { SessionAttention } from "#src/features/session/model/session-attention.ts";
import { rpcFetch } from "./stream-fetch";

// All client state belongs to one authenticated Connection. No credentials or
// conversation cache are persisted in localStorage. Drafts use tab-local storage.
export class Connection {
  readonly clientId = crypto.randomUUID();
  readonly transport;
  readonly store;
  readonly queries;
  readonly projects;
  // Not an entity service, so it is not in the resource store's catalog: the
  // installation is files on the manager. See docs/plans/service-boundaries.md.
  readonly installation;
  readonly sessions;
  readonly accounts;
  readonly bindings;
  readonly tasks;
  readonly taskCatalog;
  readonly inventory;
  readonly attention;
  readonly conversations;
  // Original paste bodies stay in this authenticated connection's memory, like drafts.
  readonly pastes = new Map<string, ComposerPaste>();
  readonly drafts;
  readonly editors = new Map<string, EditorState>();
  readonly inProcess: boolean;
  constructor(
    readonly baseUrl = location.origin,
    transport?: Transport,
    // How a file body reaches this connection's server, for a connection whose
    // server is not at the end of a socket. Absent means the request goes out
    // as itself; see composer-upload's Transfer.
    readonly transfer?: Transfer,
  ) {
    this.inProcess = !!transport;
    this.drafts = new SessionDrafts(baseUrl, this.pastes);
    this.transport =
      transport ??
      createConnectTransport({
        baseUrl,
        fetch: rpcFetch,
      });
    this.store = Store.open(entities, {
      name: "cxz",
      identity: crypto.randomUUID(),
    });
    this.queries = new Queries(this.store, this.transport, entities);
    this.projects = createClient(ProjectService, this.transport);
    this.installation = createClient(InstallationService, this.transport);
    this.sessions = createClient(SessionService, this.transport);
    this.accounts = createClient(AccountService, this.transport);
    this.bindings = createClient(AuthBindingService, this.transport);
    this.tasks = createClient(TaskService, this.transport);
    this.taskCatalog = new TaskCatalog(this.tasks);
    this.attention = new SessionAttention(this.sessions, baseUrl);
    this.conversations = new ConversationCache(this);
    this.inventory = new ResourceInventory(
      this.projects,
      this.sessions,
      this.store,
    );
  }
  dispose() {
    this.taskCatalog.dispose();
    this.conversations.dispose();
    this.attention.dispose();
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
