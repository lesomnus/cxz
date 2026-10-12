import { createRouterTransport, ConnectError, Code } from "@connectrpc/connect";
import { AccountService } from "#gen/cxz/account_svc_pb";
import { ProjectService, AuxKind } from "#gen/cxz/project_svc_pb";
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
  const accounts = [
    {
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
        const account = {
          alias: request.alias,
          name: request.name,
          agent: request.agent,
          authBackend:
            request.agent === "claude"
              ? "project-local-oauth"
              : "brokered-access-token",
        };
        accounts.push(account);
        return account;
      },
    });
    router.service(ProjectService, {
      auxConfig: () => ({ profiles: config }),
      auxSetConfig: (request) => {
        config.splice(
          0,
          config.length,
          ...request.profiles.map((p) => ({ ...p, effort: p.effort })),
        );
        return { profiles: config };
      },
      auxModels: () => ({
        models: [
          { id: "demo-model", name: "Demo model", efforts: ["low", "high"] },
        ],
      }),
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
