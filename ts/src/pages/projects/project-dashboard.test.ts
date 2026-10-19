import { describe, expect, it, vi } from "vitest";
import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { SessionSchema, SessionEventSchema } from "#gen/cxz/session_pb";
import { ProjectDownloadReplySchema } from "#gen/cxz/project_svc_pb";
import {
  projectBranch,
  recentSession,
  sessionCounts,
  type DashboardConnection,
} from "./project-dashboard";

const project = () =>
  create(ProjectSchema, {
    runtimeId: "chosen-project",
    workspace: "/host/source",
    status: { state: "running", remoteWorkspace: "/workspace/app" },
  });
function downloads(files: Record<string, string | Error>) {
  const paths: string[] = [];
  const client = {
    projects: {
      download: vi.fn(async function* (
        request: { ref?: { key: { value?: unknown } }; path: string },
        options: { signal: AbortSignal },
      ) {
        expect(request.ref?.key.value).toBe("chosen-project");
        expect(options.signal).toBeInstanceOf(AbortSignal);
        paths.push(request.path);
        const body = files[request.path];
        if (body instanceof Error) throw body;
        if (body === undefined)
          throw new ConnectError("missing", Code.NotFound);
        const data = new TextEncoder().encode(body);
        yield create(ProjectDownloadReplySchema, {
          data,
          totalSize: BigInt(data.length),
        });
      }),
    },
  } as unknown as DashboardConnection;
  return { client, paths };
}
const signal = () => new AbortController().signal;
describe("project Git metadata", () => {
  it("reads the selected container's branch without using its host workspace", async () => {
    const { client, paths } = downloads({
      "/workspace/app/.git/HEAD": "ref: refs/heads/feature/dashboard\n",
    });
    expect(await projectBranch(client, project(), signal())).toEqual({
      state: "branch",
      value: "feature/dashboard",
    });
    expect(paths).toEqual(["/workspace/app/.git/HEAD"]);
  });
  it("resolves relative worktree pointers and detached HEADs", async () => {
    const { client, paths } = downloads({
      "/workspace/app/.git": "gitdir: ../repo/.git/worktrees/app\n",
      "/workspace/repo/.git/worktrees/app/HEAD": "a".repeat(40),
    });
    expect(await projectBranch(client, project(), signal())).toEqual({
      state: "detached",
      value: "aaaaaaaa",
    });
    expect(paths.at(-1)).toBe("/workspace/repo/.git/worktrees/app/HEAD");
  });
  it("distinguishes a missing repository from denied access", async () => {
    expect(
      await projectBranch(downloads({}).client, project(), signal()),
    ).toEqual({ state: "missing" });
    await expect(
      projectBranch(
        downloads({
          "/workspace/app/.git/HEAD": new ConnectError(
            "denied",
            Code.PermissionDenied,
          ),
        }).client,
        project(),
        signal(),
      ),
    ).rejects.toThrow("denied");
  });
  it("does not read stopped projects or guess an unknown container path", async () => {
    const { client, paths } = downloads({});
    const p = project();
    p.status!.state = "stopped";
    expect(await projectBranch(client, p, signal())).toEqual({
      state: "offline",
    });
    p.status!.state = "running";
    p.status!.remoteWorkspace = "";
    expect(await projectBranch(client, p, signal())).toEqual({
      state: "unavailable",
    });
    expect(paths).toEqual([]);
  });
  it("bounds metadata reads and respects cancellation", async () => {
    await expect(
      projectBranch(
        downloads({ "/workspace/app/.git/HEAD": "x".repeat(5000) }).client,
        project(),
        signal(),
      ),
    ).rejects.toThrow("read limit");
    const controller = new AbortController();
    controller.abort();
    await expect(
      projectBranch(
        downloads({ "/workspace/app/.git/HEAD": "ref: refs/heads/main" })
          .client,
        project(),
        controller.signal,
      ),
    ).rejects.toThrow();
  });
});

it("counts stopped, interrupted and failed separately from ready and waiting sessions", () => {
  const sessions = [
    "working",
    "running",
    "waiting_input",
    "waiting",
    "idle",
    "stopped",
    "interrupted",
    "failed",
    "",
  ].map((state) => create(SessionSchema, { status: { state } }));
  expect(sessionCounts(sessions)).toEqual({
    total: 9,
    working: 2,
    waiting: 2,
    idle: 1,
    stopped: 3,
    unknown: 1,
  });
});
it("does not label creation settings or an earlier run's catalog as current model", () => {
  const s = create(SessionSchema, {
    model: "creation-model",
    status: { runId: "new-run" },
  });
  const event = (runId: string) =>
    create(SessionEventSchema, {
      runId,
      seq: 1n,
      kind: "models",
      payload: new TextEncoder().encode(
        JSON.stringify({
          effective_model: "runtime-model",
          effective_effort: "high",
        }),
      ),
    });
  expect(recentSession(s, [], []).info.model).toBe("");
  expect(recentSession(s, [], [event("old-run")]).info.model).toBe("");
  const unreported = event("new-run");
  unreported.payload = new TextEncoder().encode(
    JSON.stringify({ models: [{ id: "catalog-only" }] }),
  );
  expect(recentSession(s, [], [unreported]).info.model).toBe("");
  expect(recentSession(s, [], [event("new-run")]).info).toMatchObject({
    model: "runtime-model",
    effort: "high",
  });
});
it("keeps errors distinct from failed tasks and bounds activity previews", () => {
  const events = Array.from({ length: 15 }, (_, i) =>
    create(SessionEventSchema, {
      seq: BigInt(i + 1),
      kind: "assistant",
      text: `Response ${i}`,
    }),
  );
  events.push(
    create(SessionEventSchema, { seq: 16n, kind: "tool", text: "Bash failed" }),
    create(SessionEventSchema, {
      seq: 17n,
      kind: "error",
      text: "Provider failed",
    }),
  );
  const result = recentSession(create(SessionSchema), events, []);
  expect(result.activity).toHaveLength(6);
  expect(result.activity[0].text).toBe("Response 14");
  expect(result.errors.map((e) => e.text)).toEqual(["Provider failed"]);
});
