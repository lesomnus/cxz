import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { isFieldSet } from "@bufbuild/protobuf";
import type { Account } from "#gen/cxz/account_pb";
import { createRouterTransport, ConnectError, Code } from "@connectrpc/connect";
import {
  AccountService,
  AccountPatchRequestSchema,
  AccountAuthStatus_State as AuthState,
} from "#gen/cxz/account_svc_pb";
import { AuxKind, InstallationService } from "#gen/cxz/installation_pb";
import { SessionService } from "#gen/cxz/session_svc_pb";
import { Connection } from "#src/shared/api/connection.ts";
import { projectFixtureId } from "./project-id";

// This fixture uses the same typed clients as production without credentials,
// Docker or a provider. Each story owns its own editable state.
export function resourceFixture({
  eraseFailures = 0,
  usageDelayMs = 0,
  usageFailures = 0,
  longAccountDetails = false,
  editFailures = 0,
  quotaRemaining = 64,
  onRead,
}: {
  eraseFailures?: number;
  usageDelayMs?: number;
  usageFailures?: number;
  longAccountDetails?: boolean;
  editFailures?: number;
  quotaRemaining?: number | null;
  onRead?: (kind: "accounts" | "auth" | "usage" | "quota") => void;
} = {}) {
  const accounts: Pick<
    Account,
    | "id"
    | "alias"
    | "name"
    | "desc"
    | "agent"
    | "authBackend"
    | "dateCreated"
    | "dateUpdated"
  >[] = [
    {
      id: projectFixtureId(10),
      dateCreated: timestampFromDate(new Date("2026-10-20T00:00:00Z")),
      dateUpdated: timestampFromDate(new Date("2026-10-20T00:00:00Z")),
      alias: "work",
      name: "Work",
      desc: "",
      agent: "codex",
      authBackend: "brokered-access-token",
    },
    {
      id: projectFixtureId(11),
      dateUpdated: timestampFromDate(new Date("2026-10-20T00:00:00Z")),
      alias: "personal",
      name: "Personal",
      desc: "",
      agent: "claude",
      authBackend: "project-local-oauth",
    },
  ];
  if (longAccountDetails) {
    accounts[0].name =
      "Work account with a very long display name that must not resize the panel";
    accounts[0].authBackend =
      "brokered-access-token-with-a-very-long-backend-description";
    accounts[0].desc = "Account description. ".repeat(40);
  }
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
      list: () => {
        onRead?.("accounts");
        return { items: accounts };
      },
      get: (request) => {
        const account = accounts.find((a) =>
          request.ref?.key.case === "id"
            ? a.id.toString() === request.ref.key.value.toString()
            : a.alias === request.ref?.key.value,
        );
        if (!account)
          throw new ConnectError("Account not found", Code.NotFound);
        return account;
      },
      patch: (request) => {
        if (editFailures > 0) {
          editFailures--;
          throw new ConnectError(
            "Account save failed. Try again.",
            Code.Unavailable,
          );
        }
        const account = accounts.find((a) =>
          request.ref?.key.case === "id"
            ? a.id.toString() === request.ref.key.value.toString()
            : a.alias === request.ref?.key.value,
        );
        if (!account)
          throw new ConnectError("Account not found", Code.NotFound);
        if (
          !request.dateUpdated ||
          request.dateUpdated.seconds !== account.dateUpdated?.seconds ||
          request.dateUpdated.nanos !== account.dateUpdated?.nanos
        )
          throw new ConnectError(
            "Account changed. Refresh and try again.",
            Code.FailedPrecondition,
          );
        if (isFieldSet(request, AccountPatchRequestSchema.field.alias)) {
          if (accounts.some((a) => a !== account && a.alias === request.alias))
            throw new ConnectError(
              "Account alias is already in use",
              Code.AlreadyExists,
            );
          if (signedIn.delete(account.alias)) signedIn.add(request.alias);
          account.alias = request.alias;
        }
        if (isFieldSet(request, AccountPatchRequestSchema.field.name))
          account.name = request.name;
        account.dateUpdated = {
          ...account.dateUpdated!,
          nanos: account.dateUpdated!.nanos + 1,
        };
        return account;
      },
      add: (request) => {
        if (accounts.some((account) => account.alias === request.alias))
          throw new ConnectError("Account already exists", Code.AlreadyExists);
        const account = {
          id: projectFixtureId(accounts.length + 20),
          alias: request.alias,
          name: request.name || request.alias,
          desc: "",
          dateUpdated: timestampFromDate(new Date()),
          agent: request.agent,
          authBackend:
            request.agent === "claude"
              ? "project-local-oauth"
              : "brokered-access-token",
        };
        accounts.push(account);
        return account;
      },
      status: (ref) => {
        onRead?.("auth");
        return authStatus(ref.key.case === "alias" ? ref.key.value : "");
      },
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
      erase: (ref) => {
        if (eraseFailures > 0) {
          eraseFailures--;
          throw new ConnectError(
            "Session cleanup failed. Retry to finish erasing this account.",
            Code.Unavailable,
          );
        }
        const index = accounts.findIndex((a) =>
          ref.key.case === "alias"
            ? a.alias === ref.key.value
            : ref.key.case === "id" &&
              a.id.toString() === ref.key.value.toString(),
        );
        if (index < 0) return { erased: false };
        signedIn.delete(accounts[index].alias);
        accounts.splice(index, 1);
        return { erased: true };
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
      transcript: (request) => {
        onRead?.("quota");
        const id = request.ref?.key.value;
        if (
          quotaRemaining === null ||
          !["demo", "demo-review"].includes(String(id))
        )
          return {};
        const older = id === "demo";
        const remaining = older ? 12 : quotaRemaining;
        return {
          metadata: [
            {
              seq: older ? 1000n : 1n,
              timeMs: BigInt(Date.now() - (older ? 60000 : 1000)),
              kind: "usage",
              text: "account/rateLimits/updated",
              payload: new TextEncoder().encode(
                JSON.stringify({
                  rateLimits: {
                    primary: {
                      usedPercent: 100 - remaining,
                      windowDurationMins: 300,
                      resetsAt:
                        Math.floor(Date.now() / 1000) + 2 * 3600 + 25 * 60,
                    },
                    secondary: {
                      usedPercent: Math.min(10, 100 - remaining),
                      windowDurationMins: 10080,
                    },
                  },
                }),
              ),
            },
          ],
        };
      },
      conversationStats: async (request) => {
        onRead?.("usage");
        if (usageDelayMs)
          await new Promise((resolve) => setTimeout(resolve, usageDelayMs));
        if (usageFailures > 0) {
          usageFailures--;
          throw new ConnectError(
            "Conversation statistics temporarily unavailable. ".repeat(10),
            Code.Unavailable,
          );
        }
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
