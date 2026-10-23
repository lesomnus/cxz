import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import type { Account } from "#gen/cxz/account_pb";
import { createRouterTransport, ConnectError, Code } from "@connectrpc/connect";
import {
  AccountService,
  AccountAuthStatus_State as AuthState,
} from "#gen/cxz/account_svc_pb";
import { AuxKind, InstallationService } from "#gen/cxz/installation_pb";
import { SessionService } from "#gen/cxz/session_svc_pb";
import { Connection } from "#src/shared/api/connection.ts";

// This fixture uses the same typed clients as production without credentials,
// Docker or a provider. Each story owns its own editable state.
export function resourceFixture() {
  let revision = "1",
    content =
      "# Project memory\n\nKeep decisions here.\n\n```ts\nconst shared = true;\n```";
  const memory = {
    id: "own",
    name: "Project notes",
    session: "demo",
    agent: "codex",
    snapshot: false,
  };
  const accounts: Pick<
    Account,
    "alias" | "name" | "agent" | "authBackend" | "dateCreated"
  >[] = [
    {
      dateCreated: timestampFromDate(new Date("2026-10-20T00:00:00Z")),
      alias: "work",
      name: "Work",
      agent: "codex",
      authBackend: "brokered-access-token",
    },
    {
      alias: "personal",
      name: "Personal",
      agent: "claude",
      authBackend: "project-local-oauth",
    },
  ];
  const signedIn = new Set(["work"]);
  const authStatus = (alias: string) => {
    const account = accounts.find((account) => account.alias === alias);
    if (!account) throw new ConnectError("Account not found", Code.NotFound);
    if (account.authBackend === "project-local-oauth")
      return {
        state: AuthState.SESSION_SCOPED,
        scope: "session",
        canLogin: false,
        canLogout: false,
        credentialPresent: false,
        vendorVerified: false,
        message:
          "Login belongs to each session. Manage it from that session's details.",
      };
    const ready = signedIn.has(alias);
    return {
      state: ready ? AuthState.READY : AuthState.LOGIN_REQUIRED,
      scope: "account",
      credentialPresent: ready,
      vendorVerified: false,
      canLogin: !ready,
      canLogout: ready,
      message: ready
        ? "Saved login is present. Provider validity has not been checked."
        : "Log in to this account to use it.",
    };
  };
  const config = [
    {
      kind: AuxKind.SUMMARY,
      enabled: true,
      account: "work",
      model: "demo-model",
      effort: "low",
    },
  ];
  const transport = createRouterTransport((router) => {
    router.service(AccountService, {
      list: () => ({ items: accounts }),
      add: (request) => {
        if (accounts.some((account) => account.alias === request.alias))
          throw new ConnectError("Account already exists", Code.AlreadyExists);
        const account = {
          alias: request.alias,
          name: request.name || request.alias,
          agent: request.agent,
          authBackend:
            request.agent === "claude"
              ? "project-local-oauth"
              : "brokered-access-token",
        };
        accounts.push(account);
        return account;
      },
      status: (ref) =>
        authStatus(ref.key.case === "alias" ? ref.key.value : ""),
      logout: (ref) => {
        const alias = ref.key.case === "alias" ? ref.key.value : "";
        const status = authStatus(alias);
        if (status.scope !== "account")
          throw new ConnectError(
            "Account uses session logins",
            Code.FailedPrecondition,
          );
        signedIn.delete(alias);
        return authStatus(alias);
      },
      models: () => ({
        models: [
          { id: "demo-model", name: "Demo model", efforts: ["low", "high"] },
        ],
      }),
    });
    router.service(InstallationService, {
      agents: () => ({ profiles: config }),
      saveAgents: (request) => {
        config.splice(
          0,
          config.length,
          ...request.profiles.map((p) => ({ ...p, effort: p.effort })),
        );
        return { profiles: config };
      },
    });
    router.service(SessionService, {
      libraryMemories: () => ({
        memories: [
          memory,
          {
            ...memory,
            id: "snapshot",
            name: "Saved checkpoint",
            session: "previous",
            snapshot: true,
          },
        ],
      }),
      libraryMemory: (request) => ({
        memory:
          request.id === "snapshot"
            ? { ...memory, id: "snapshot", snapshot: true }
            : memory,
        documents: [
          { name: "notes.md", revision, size: BigInt(content.length) },
        ],
      }),
      libraryDocument: (request) => ({
        memory,
        revision,
        content:
          request.id === "snapshot"
            ? "# Saved checkpoint\nRead only snapshot."
            : content,
      }),
      writeLibraryDocument: (request) => {
        if (request.revision && request.revision !== revision)
          throw new ConnectError("Document revision changed", Code.Aborted);
        content = request.content;
        revision = String(Number(revision) + 1);
        return { memory, content, revision };
      },
      snapshotLibraryMemory: () => ({ memory: { ...memory, snapshot: true } }),
      forkLibraryMemory: () => ({ memory }),
      renameLibraryMemory: (request) => {
        memory.name = request.name;
        return { memory };
      },
      conversationStats: () => ({
        complete: true,
        total: {
          finishedTurns: 12n,
          inputTokens: 4800n,
          outputTokens: 2300n,
          cacheReadTokens: 12000n,
          cacheWriteTokens: 400n,
          tokenReportedTurns: 12n,
          reportedCostUsd: 0.42,
          costReportedTurns: 12n,
        },
      }),
      background: () => ({
        data: new TextEncoder().encode(
          JSON.stringify({
            run: {
              Tasks: {
                build: {
                  task_id: "build",
                  description: "Build and verify",
                  active: true,
                  status: "working",
                },
              },
            },
          }),
        ),
      }),
    });
  });
  return new Connection(location.origin, transport);
}
