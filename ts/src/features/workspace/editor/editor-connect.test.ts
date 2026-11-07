import { afterEach, expect, it, vi } from "vitest";
import type { Connection } from "#src/shared/api/connection.ts";
import { ClientErrors } from "#src/shared/errors/client-errors.ts";
import { connectWorkspaceEditor } from "./editor-connect";

afterEach(() => vi.unstubAllGlobals());
it("retains native HTTP failures during a retry, then resolves only the matching editor connection", async () => {
  const errors = new ClientErrors();
  const c = {
    baseUrl: "https://host.invalid",
    inProcess: false,
    errors,
  } as Connection;
  const signal = new AbortController().signal;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("Editor unavailable", { status: 503 })),
  );
  await expect(
    connectWorkspaceEditor(c, "project-one", signal),
  ).rejects.toThrow("Editor unavailable");
  expect(errors.snapshot()[0]).toMatchObject({
    status: "active",
    origin: {
      kind: "http",
      method: "POST",
      path: "/editor/connect",
      status: 503,
    },
    subject: { kind: "project", id: "project-one" },
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ workspace: "/workspace" })),
  );
  await connectWorkspaceEditor(c, "project-two", signal);
  expect(errors.snapshot()[0].status).toBe("active");
  let finish!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    ),
  );
  const retry = connectWorkspaceEditor(c, "project-one", signal);
  expect(errors.snapshot()[0].status).toBe("active");
  finish(Response.json({ workspace: "/workspace" }));
  await retry;
  expect(errors.snapshot()[0]).toMatchObject({
    status: "resolved",
    source: "http",
  });
});
