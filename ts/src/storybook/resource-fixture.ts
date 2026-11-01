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
      conversationStats: (request) => {
        const toMs = request.toMs || BigInt(Date.parse("2026-11-01T18:00:00Z"));
        const fromMs = request.fromMs || toMs - 30n * 86400000n;
        const days = [
          ["2026-08-12", 6, 900, 200, 0.21],
          ["2026-10-03", 1, 150, 0, 0.03],
          ["2026-10-09", 2, 400, 100, 0.07],
          ["2026-10-14", 1, 150, 0, 0.03],
          ["2026-10-24", 4, 800, 200, 0.14],
          ["2026-10-27", 1, 200, 0, 0.03],
          ["2026-10-31", 2, 400, 50, 0.08],
          ["2026-11-01", 1, 200, 50, 0.04],
        ] as const;
        const recorded = days
          .filter(([date]) => {
            const start = Date.parse(`${date}T00:00:00Z`);
            return start < Number(toMs) && start + 86400000 > Number(fromMs);
          })
          .map(([date, turns, output, cacheWrite, cost]) => ({
            date,
            values: {
              finishedTurns: BigInt(turns),
              inputTokens: BigInt(Number(turns) * 400),
              outputTokens: BigInt(output),
              cacheReadTokens: BigInt(Number(turns) * 1000),
              cacheWriteTokens: BigInt(cacheWrite),
              tokenReportedTurns: BigInt(turns),
              costReportedTurns: BigInt(turns),
              reportedCostUsd: Number(cost),
            },
          }));
        const total = {
          finishedTurns: 0n,
          inputTokens: 0n,
          outputTokens: 0n,
          cacheReadTokens: 0n,
          cacheWriteTokens: 0n,
          tokenReportedTurns: 0n,
          costReportedTurns: 0n,
          reportedCostUsd: 0,
        };
        for (const { values } of recorded) {
          for (const key of [
            "finishedTurns",
            "inputTokens",
            "outputTokens",
            "cacheReadTokens",
            "cacheWriteTokens",
            "tokenReportedTurns",
            "costReportedTurns",
          ] as const)
            total[key] += values[key];
          total.reportedCostUsd += values.reportedCostUsd;
        }
        total.reportedCostUsd = Math.round(total.reportedCostUsd * 100) / 100;
        return {
          fromMs,
          toMs,
          generatedMs: toMs,
          complete: true,
          total,
          days: recorded,
        };
      },
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
