import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { Button } from "@lesomnus/nox";
import { AccountService } from "#gen/cxz/account_svc_pb";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import { SessionService } from "#gen/cxz/session_svc_pb";
import { Connection } from "#src/shared/api/connection.ts";
import { ClientErrors } from "#src/shared/errors/client-errors.ts";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { createWorkspaceRouter } from "./router";
import { Workspace } from "./workspace-shell";
import { EditableValue } from "#src/shared/components/editable-value.tsx";

// Real clients/diagnostic transport and the real workspace sidebar. The fixture
// controls replies, rather than adding error records directly from buttons.
function makeFixture() {
  const errors = new ClientErrors({
    storage: localStorage,
    storageKey: "cxz.error-center-preview.v1",
  });
  let mode: "fail" | "success" | "wait" = "fail";
  let streamFails = true;
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
function Preview() {
  const [fixture] = useState(makeFixture);
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
        view: () => <Controls fixture={fixture} />,
      },
      createMemoryHistory({ initialEntries: ["/c/_/account"] }),
    ),
  );
  useEffect(() => {
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
