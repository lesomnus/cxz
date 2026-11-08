import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { create } from "@bufbuild/protobuf";
import { SessionSchema, SessionEventSchema } from "#gen/cxz/session_pb";
import { ConversationLoading } from "#src/features/session/conversation/conversation-loading.tsx";
import { SessionsPage } from "#src/pages/sessions/sessions-page.tsx";
import { errorNotificationOptions } from "#src/shared/errors/error-notification.ts";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { Button } from "@lesomnus/nox";
import { AccountService } from "#gen/cxz/account_svc_pb";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import { SessionService } from "#gen/cxz/session_svc_pb";
import { Connection } from "#src/shared/api/connection.ts";
import type { ErrorContext } from "#src/shared/errors/client-errors.ts";
import { ClientErrors } from "#src/shared/errors/client-errors.ts";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { ErrorsPage } from "#src/pages/errors/errors-page.tsx";
import { useWorkspaceRoute, createWorkspaceRouter } from "./router";
import { Workspace } from "./workspace-shell";
import { EditableValue } from "#src/shared/components/editable-value.tsx";

// Real clients/diagnostic transport and the real workspace sidebar. The fixture
// controls replies, rather than adding error records directly from buttons.
type ReadFailure = "lookup" | "session" | "history";
function makeFixture(readFailure?: ReadFailure) {
  let failRead = true;
  const errors = new ClientErrors({
    storage: localStorage,
    storageKey: readFailure
      ? `cxz.attended-error-preview.v1:${readFailure}`
      : "cxz.error-center-preview.v1",
  });
  let mode: "fail" | "success" | "wait" = "fail";
  let streamFails = !readFailure;
  let release: (() => void) | undefined;
  const idle = (signal: AbortSignal) =>
    new Promise<void>((resolve) => {
      if (signal.aborted) resolve();
      else signal.addEventListener("abort", () => resolve(), { once: true });
    });
  const transport = createRouterTransport((router) => {
    router.service(AccountService, {
      get: async (request, context) => {
        if (request.ref?.key.value === "other") return {};
        if (mode === "fail")
          throw new ConnectError(
            "Account service is offline",
            Code.Unavailable,
          );
        if (mode === "wait")
          await new Promise<void>((resolve) => {
            release = resolve;
            context.signal.addEventListener("abort", () => resolve(), {
              once: true,
            });
          });
        return {};
      },
    });
    router.service(ProjectService, {
      list: () => ({}),
      watch: async function* (_, context) {
        yield { snapshotComplete: true };
        await idle(context.signal);
      },
    });
    router.service(SessionService, {
      get: () => {
        if (readFailure && readFailure !== "history" && failRead)
          throw new ConnectError("session not found", Code.NotFound);
        return create(SessionSchema, {
          id: new Uint8Array(16).fill(2),
          runtimeId: "read-preview",
          alias: "read-preview",
          status: { state: "stopped", runId: "run", lastSeq: 1n },
        });
      },
      transcript: () => {
        if (readFailure === "history" && failRead)
          throw new ConnectError(
            "Conversation history is unavailable",
            Code.Unavailable,
          );
        return {
          events: [
            create(SessionEventSchema, {
              seq: 1n,
              kind: "assistant",
              text: "Recovered conversation",
              runId: "run",
            }),
          ],
          snapshotSeq: 1n,
        };
      },
      history: () => ({}),
      rows: async function* (_, context) {
        yield {};
        await idle(context.signal);
      },
      list: () => ({}),
      watch: async function* (_, context) {
        yield { snapshotComplete: true };
        await idle(context.signal);
      },
      events: async function* () {
        if (streamFails)
          throw new ConnectError(
            "Conversation stream disconnected",
            Code.Unavailable,
          );
        yield { heartbeat: true };
      },
    });
  });
  const c = new Connection(
    "https://errors-preview.invalid",
    transport,
    undefined,
    undefined,
    errors,
  );
  return {
    c,
    errors,
    recoverRead: () => {
      failRead = false;
    },
    quietRequest: () =>
      c.accounts.get(
        { ref: { key: { case: "alias", value: "quiet" } } },
        errorNotificationOptions(false),
      ),
    request: (value: typeof mode, alias = "work", signal?: AbortSignal) => {
      mode = value;
      return c.accounts.get(
        { ref: { key: { case: "alias", value: alias } } },
        { signal },
      );
    },
    complete: () => release?.(),
    stream: async (fail: boolean) => {
      streamFails = fail;
      for await (const _ of c.sessions.events({
        ref: { key: { case: "alias", value: "seal" } },
      })) {
        /* first heartbeat recovers */
      }
    },
  };
}
function Controls({ fixture: f }: { fixture: ReturnType<typeof makeFixture> }) {
  const [error, setError] = useState("");
  const [pending, setPending] = useState<AbortController>();
  const run = async (call: () => Promise<unknown>) => {
    setError("");
    try {
      await call();
    } catch (e) {
      setError(String(e));
    }
  };
  return (
    <main
      className="resource-view"
      style={{ padding: 24, gridColumn: "2 / -1" }}
    >
      <h2>API error recovery</h2>
      <p>
        Failures appear above Profile. Retrying or canceling does not mean
        recovered.
      </p>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", maxWidth: 700 }}>
        <Button onClick={() => void run(() => f.request("fail"))}>
          Fail request
        </Button>
        <Button
          disabled={!!pending}
          onClick={() => {
            const controller = new AbortController();
            setPending(controller);
            void run(() =>
              f.request("wait", "work", controller.signal),
            ).finally(() => setPending(undefined));
          }}
        >
          Start retry
        </Button>
        <Button disabled={!pending} onClick={() => f.complete()}>
          Complete retry
        </Button>
        <Button disabled={!pending} onClick={() => pending?.abort()}>
          Cancel retry
        </Button>
        <Button onClick={() => void run(() => f.request("success"))}>
          Recover request
        </Button>
        <Button onClick={() => void run(() => f.request("success", "other"))}>
          Succeed for another account
        </Button>
        <Button onClick={() => void run(() => f.stream(true))}>
          Fail stream
        </Button>
        <Button onClick={() => void run(() => f.stream(false))}>
          Recover stream
        </Button>
      </div>
      <ErrorNotice error={error} />
      <EditableValue
        label="Account alias"
        value="work"
        save={async () => {
          await f.request("fail");
        }}
        validate={(value) => (value ? undefined : "Alias is required")}
      />
      <ErrorNotice error="Example local input validation" />
    </main>
  );
}
function ReadPane({ fixture }: { fixture: ReturnType<typeof makeFixture> }) {
  const feed = useMemo(
    () => fixture.c.conversations.get("read-preview"),
    [fixture],
  );
  const state = useSyncExternalStore(feed.subscribe, feed.snapshot);
  return (
    <section
      aria-label="Conversation read"
      style={{ position: "relative", minHeight: 320 }}
    >
      {state.loading ? (
        <ConversationLoading {...state.loading} />
      ) : (
        state.events.map((event) => <p key={String(event.seq)}>{event.text}</p>)
      )}
    </section>
  );
}
function ReadControls({
  fixture,
  mode,
}: {
  fixture: ReturnType<typeof makeFixture>;
  mode: ReadFailure;
}) {
  const [shown, setShown] = useState(true);
  return (
    <main
      style={{
        padding: 24,
        gridColumn: "2 / -1",
        display: "flex",
        flexDirection: "column",
      }}
    >
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <Button onClick={() => setShown(!shown)}>
          {shown ? "Hide conversation" : "Show conversation"}
        </Button>
        {mode !== "lookup" && (
          <Button
            onClick={() => {
              fixture.recoverRead();
              const feed = fixture.c.conversations.get("read-preview");
              feed.pause();
              feed.resume();
            }}
          >
            Recover conversation
          </Button>
        )}
        <Button
          onClick={() => fixture.request("fail", "background").catch(() => {})}
        >
          Fail background request
        </Button>
        <Button onClick={() => fixture.quietRequest().catch(() => {})}>
          Fail silent request
        </Button>
      </div>
      {shown &&
        (mode === "lookup" ? (
          <SessionsPage c={fixture.c} id="missing" projects={[]} />
        ) : (
          <ReadPane fixture={fixture} />
        ))}
    </main>
  );
}
function Preview({
  page = false,
  overview = false,
  readFailure,
}: {
  page?: boolean;
  overview?: boolean;
  readFailure?: ReadFailure;
}) {
  const [fixture] = useState(() => makeFixture(readFailure));
  const [router] = useState(() =>
    createWorkspaceRouter(
      {
        shell: ({ children }) => (
          <Workspace
            connection={fixture.c}
            logout={async () => {
              throw Error("Sign out is unavailable");
            }}
          >
            {children}
          </Workspace>
        ),
        view: () =>
          useWorkspaceRoute().resource === "errors" ? (
            <ErrorsPage store={fixture.errors} />
          ) : readFailure ? (
            <ReadControls fixture={fixture} mode={readFailure} />
          ) : (
            <Controls fixture={fixture} />
          ),
      },
      createMemoryHistory({
        initialEntries: [page ? "/c/_/errors" : "/c/_/account"],
      }),
    ),
  );
  useEffect(() => {
    if (!fixture.errors.snapshot().length) {
      if (overview) {
        const examples = [
          [
            "The project container is unavailable. Start the container to reconnect its sessions.",
            "Project runtime",
            "Project: Web workspace",
          ],
          [
            "Conversation stream disconnected while waiting for the next response.",
            "cxz.SessionService/Events",
            "Session: seal",
          ],
          [
            "Account service is offline",
            "cxz.AccountService/Get",
            "Account: work",
          ],
          [
            "File upload failed before the server confirmed receipt.",
            "Upload",
            "Project: API workspace",
          ],
          [
            "Memory revision changed. Read the current revision before saving the document.",
            "cxz.MemoryService/Patch",
            "Memory: Architecture decisions",
          ],
          [
            "A background operation failed before completion.",
            "Background refresh",
            "",
          ],
          [
            "Provider login was interrupted",
            "Provider login",
            "Account: personal",
          ],
          [
            "Project status could not be refreshed",
            "cxz.ProjectService/Get",
            "Project: API workspace",
          ],
          [
            "Editor preview could not be rendered",
            "Markdown preview",
            "Session: review",
          ],
        ];
        const contexts: ErrorContext[] = [
          {
            source: "resource",
            origin: { kind: "runtime", operation: "project-status" },
            subject: { kind: "project", name: "Web workspace" },
          },
          {
            source: "rpc",
            origin: {
              kind: "rpc",
              service: "cxz.SessionService",
              method: "Events",
            },
            subject: {
              kind: "session",
              name: "Release review",
              alias: "seal",
              runtimeId: "89502035e952a2c227802d29",
            },
          },
          {
            source: "rpc",
            origin: {
              kind: "rpc",
              service: "cxz.AccountService",
              method: "Get",
            },
            subject: { kind: "account", alias: "work" },
          },
          {
            source: "http",
            origin: {
              kind: "http",
              method: "POST",
              path: "/upload",
              status: 503,
            },
            subject: { kind: "file", name: "design.png" },
          },
          {
            source: "rpc",
            origin: {
              kind: "rpc",
              service: "cxz.MemoryService",
              method: "Patch",
            },
            subject: { kind: "memory", name: "Architecture decisions" },
          },
          {
            source: "client",
            origin: { kind: "client", operation: "unhandled-rejection" },
          },
          {
            source: "http",
            origin: {
              kind: "connection",
              purpose: "provider-login",
              transport: "websocket",
              closeCode: 1006,
            },
            subject: { kind: "account", alias: "personal" },
          },
          {
            source: "rpc",
            origin: {
              kind: "rpc",
              service: "cxz.ProjectService",
              method: "Get",
            },
            subject: { kind: "project", name: "API workspace" },
          },
          {
            source: "client",
            origin: { kind: "client", operation: "exception" },
          },
        ];
        examples.forEach(([message, method, target], index) => {
          const context = {
            ...contexts[index],
            method,
            target,
            connection: "https://workspace.example.com",
            code: index === 4 ? "Aborted" : "Unavailable",
            request: crypto.randomUUID(),
          };
          const id = fixture.errors.report(
            `overview-${index}`,
            message,
            context,
          );
          if (index === 0)
            fixture.errors.report(`overview-${index}`, message, context);
          if (index === 6 || index === 7)
            fixture.errors.resolve(`overview-${index}`);
          if (index === 8 && id) fixture.errors.dismiss(id);
        });
      } else if (page) void fixture.request("fail").catch(() => {});
    }
    window.addEventListener("pagehide", fixture.errors.flush);
    return () => {
      window.removeEventListener("pagehide", fixture.errors.flush);
      fixture.errors.flush();
      fixture.c.dispose();
    };
  }, [fixture]);
  return <RouterProvider router={router} />;
}
const meta = {
  title: "Workspace/Errors",
  component: Preview,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Preview>;
export default meta;
export const Recovery: StoryObj<typeof meta> = {};

export const Page: StoryObj<typeof meta> = { args: { page: true } };

export const Overview: StoryObj<typeof meta> = {
  args: { page: true, overview: true },
};

export const AttendedSession: StoryObj<typeof meta> = {
  args: { readFailure: "session" },
};
export const AttendedHistory: StoryObj<typeof meta> = {
  args: { readFailure: "history" },
};
export const AttendedLookup: StoryObj<typeof meta> = {
  args: { readFailure: "lookup" },
};
