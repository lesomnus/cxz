import { describe, expect, it, vi } from "vitest";
import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { ProjectDownloadReplySchema } from "#gen/cxz/project_svc_pb";
import { projectBranch, type DashboardConnection } from "./project-dashboard";

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
