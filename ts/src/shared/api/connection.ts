import { createRegistry } from "@bufbuild/protobuf";
import { createClient, type Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { Store } from "@lesomnus/payday/store";
import { Queries } from "@lesomnus/payday/query";
import { entities } from "#gen/entities";
import { InstallationService } from "#gen/cxz/installation_pb";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import {
  SessionService,
  SessionNotificationSchema,
} from "#gen/cxz/session_svc_pb";
import { NotificationService } from "#gen/cxz/payday/notification_pb";
import { AccountService } from "#gen/cxz/account_svc_pb";
import { AuthBindingService } from "#gen/cxz/auth_binding_svc_pb";
import { MemoryService } from "#gen/cxz/memory_svc_pb";
import { TaskService } from "#gen/cxz/task_svc_pb";
import { TaskCatalog } from "#src/features/tasks/task-catalog.ts";
import {
  openTaskCache,
  type TaskCache,
} from "#src/features/tasks/task-cache.ts";
import type { ComposerPaste } from "#src/features/session/composer/composer-pastes.ts";
import type {
  Transfer,
  UploadProgress,
} from "#src/features/session/composer/composer-upload.ts";
import type { EditorState } from "#src/features/workspace/editor/workspace-editor.tsx";
import { ResourceInventory } from "./resource-inventory";
import { SessionDrafts } from "#src/features/session/model/session-drafts.ts";
import { QuestionDrafts } from "#src/features/session/model/question-drafts.ts";
import { ConversationCache } from "#src/features/session/model/conversation-cache.ts";
import { SessionAttention } from "#src/features/session/model/session-attention.ts";
import { rpcFetch } from "./stream-fetch";
import { uploadFetch } from "./upload-fetch";
import { ReadCache } from "./read-cache";
import { createResourceSelection } from "./resource-selection";
import {
  publicUUID,
  diagnosticTransport,
} from "#src/shared/logs/rpc-diagnostics.ts";
import {
  ClientErrors,
  clientErrors,
  operationHealth,
} from "#src/shared/errors/client-errors.ts";

// All client state belongs to one authenticated Connection. No credentials or
// conversation cache are persisted in localStorage. Drafts use tab-local storage.
export class Connection {
  readonly clientId = crypto.randomUUID();
  readonly reads = new ReadCache();
  readonly selection;
  readonly transport: Transport;
  readonly store;
  readonly queries;
  readonly projects;
  // Not an entity service, so it is not in the resource store's catalog: the
  // installation is files on the manager. See docs/plans/service-boundaries.md.
  readonly installation;
  readonly sessions;
  readonly notifications;
  readonly accounts;
  readonly bindings;
  readonly tasks;
  readonly memories;
  readonly taskCatalog;
  readonly inventory: ResourceInventory;
  readonly attention;
  readonly conversations;
  // Original paste bodies stay in this authenticated connection's memory, like drafts.
  readonly pastes = new Map<string, ComposerPaste>();
  readonly drafts;
  readonly questionDrafts;
  readonly editors = new Map<string, EditorState>();
  readonly inProcess: boolean;
  readonly errors: ClientErrors;
  private taskCache?: Promise<TaskCache | undefined>;
  constructor(
    readonly baseUrl = location.origin,
    transport?: Transport,
    // How a file body reaches this connection's server, for a connection whose
    // server is not at the end of a socket. Absent means the request goes out
    // as itself; see composer-upload's Transfer.
    readonly transfer?: Transfer,
    cacheIdentity?: string,
    errors?: ClientErrors,
  ) {
    this.inProcess = !!transport;
    this.selection = createResourceSelection(
      baseUrl,
      this.inProcess ? undefined : cacheIdentity,
    );
    this.errors = errors ?? (transport ? new ClientErrors() : clientErrors());
    try {
      sessionStorage.removeItem(
        `cxz.attention.v1:${encodeURIComponent(baseUrl)}`,
      );
    } catch {
      /* Legacy tab-local read state is no longer authoritative. */
    }
    this.drafts = new SessionDrafts(baseUrl, this.pastes);
    this.questionDrafts = new QuestionDrafts(this.drafts);
    this.transport = this.observe(transport ?? this.httpTransport(rpcFetch));
    this.store = Store.open(entities, {
      name: "cxz",
      identity: cacheIdentity ?? crypto.randomUUID(),
    });
    this.queries = new Queries(this.store, this.transport, entities);
    this.projects = createClient(ProjectService, this.transport);
    this.installation = createClient(InstallationService, this.transport);
    this.sessions = createClient(SessionService, this.transport);
    this.accounts = createClient(AccountService, this.transport);
    this.bindings = createClient(AuthBindingService, this.transport);
    this.tasks = createClient(TaskService, this.transport);
    this.memories = createClient(MemoryService, this.transport);
    if (!this.inProcess && cacheIdentity)
      this.taskCache = openTaskCache(baseUrl, cacheIdentity).catch(
        () => undefined,
      );
    this.taskCatalog = new TaskCatalog(this.tasks, this.taskCache);
    this.notifications = createClient(NotificationService, this.transport);
    this.attention = new SessionAttention(this.sessions, this.notifications);
    this.conversations = new ConversationCache(this);
    this.inventory = new ResourceInventory(
      this.projects,
      this.sessions,
      this.store,
      (kind, error) =>
        operationHealth(
          this.errors,
          JSON.stringify([this.baseUrl, "subscription-health", kind]),
          {
            connection: this.baseUrl,
            method: `cxz.${kind === "projects" ? "Project" : "Session"}Service/Watch`,
            origin: { kind: "runtime", operation: "inventory" },
          },
        )(error),
    );
  }
  uploadTransport(progress?: UploadProgress): Transport {
    return progress && !this.inProcess
      ? this.observe(this.httpTransport(uploadFetch(progress)))
      : this.transport;
  }
  private httpTransport(fetcher: typeof fetch) {
    return createConnectTransport({
      baseUrl: this.baseUrl,
      jsonOptions: { registry: createRegistry(SessionNotificationSchema) },
      fetch: fetcher,
    });
  }
  private observe(transport: Transport) {
    return diagnosticTransport(transport, this.baseUrl, {
      errors: this.errors,
      ...(this.inProcess ? { write: () => {} } : {}),
      notify: ({ origin, subject }) =>
        !(
          origin?.kind === "rpc" &&
          origin.service === "cxz.SessionService" &&
          [
            "Get",
            "Transcript",
            "History",
            "Rows",
            "Events",
            "AuxStatus",
            "AuxEvents",
          ].includes(origin.method) &&
          !!subject?.runtimeId &&
          this.conversations?.attendsLoading(subject.runtimeId)
        ),
      describe: (target) => {
        const rows =
          target.kind === "session"
            ? this.inventory?.snapshot().sessions
            : target.kind === "project"
              ? this.inventory?.snapshot().projects
              : undefined;
        const resource = rows?.find(
          (row) =>
            (target.id &&
              publicUUID({ key: { case: "id", value: row.id } }) ===
                target.id) ||
            (target.runtimeId &&
              "runtimeId" in row &&
              row.runtimeId === target.runtimeId) ||
            (target.alias && row.alias === target.alias),
        );
        return resource
          ? {
              ...target,
              id: publicUUID({ key: { case: "id", value: resource.id } }),
              alias: resource.alias,
              name: resource.name,
            }
          : target;
      },
      resolve: (kind, ref): string | undefined => {
        const rows =
          kind === "session"
            ? this.inventory?.snapshot().sessions
            : this.inventory?.snapshot().projects;
        const resource = rows?.find((row) =>
          ref.key?.case === "runtimeId" && "runtimeId" in row
            ? row.runtimeId === ref.key.value
            : ref.key?.case === "alias" && row.alias === ref.key.value,
        );
        return resource
          ? publicUUID({ key: { case: "id", value: resource.id } })
          : undefined;
      },
    });
  }
  dispose() {
    this.reads.clear();
    this.taskCatalog.dispose();
    this.conversations.dispose();
    this.attention.dispose();
    void this.taskCache?.then((cache) => cache?.close()).catch(() => {});
  }
  async forgetCachedTasks() {
    this.taskCatalog.dispose();
    await this.taskCache?.then((cache) => cache?.forget()).catch(() => {});
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
  return r.headers.get("payday-identity") ?? undefined;
}
