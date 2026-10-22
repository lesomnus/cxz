import { ContainerState } from "#gen/cxz/project_pb";
import { create } from "@bufbuild/protobuf";
import { describe, expect, it, vi } from "vitest";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { stopProject } from "./project-stop";

function fixture() {
  const project = create(ProjectSchema, {
    id: new Uint8Array(16).fill(1),
    runtimeId: "project",
    alias: "web",
    listed: true,
    status: { containerState: ContainerState.RUNNING },
  });
  const stopped = create(ProjectSchema, {
    ...project,
    status: {
      ...project.status!,
      containerState: ContainerState.STOPPED,
    },
  });
  const c = {
    projects: {
      down: vi.fn().mockResolvedValue(stopped),
      erase: vi.fn(),
      remove: vi.fn(),
    },
    store: { apply: vi.fn() },
    inventory: { refresh: vi.fn() },
  };
  return {
    project,
    stopped,
    c,
    connection: c as unknown as Parameters<typeof stopProject>[0],
  };
}
describe("project stop", () => {
  it("calls Down by UUID and retains listed state without archiving or purging", async () => {
    const f = fixture();
    const result = await stopProject(f.connection, f.project);
    expect(f.c.projects.down).toHaveBeenCalledExactlyOnceWith(
      {
        ref: { key: { case: "id", value: f.project.id } },
        clientId: expect.any(String),
      },
      { timeoutMs: 120_000 },
    );
    expect(f.c.projects.erase).not.toHaveBeenCalled();
    expect(f.c.projects.remove).not.toHaveBeenCalled();
    expect(result.listed).toBe(true);
    expect(result.alias).toBe("web");
    expect(f.c.store.apply).toHaveBeenCalledWith("cxz.Project", [
      { id: f.stopped.id, value: f.stopped },
    ]);
    expect(f.c.inventory.refresh).toHaveBeenCalledOnce();
  });
  it("preserves local state on failure and does not retry", async () => {
    const f = fixture();
    f.c.projects.down.mockRejectedValue(new Error("Stop failed"));
    await expect(stopProject(f.connection, f.project)).rejects.toThrow(
      "Stop failed",
    );
    expect(f.c.projects.down).toHaveBeenCalledOnce();
    expect(f.c.store.apply).not.toHaveBeenCalled();
    expect(f.c.inventory.refresh).not.toHaveBeenCalled();
  });
});
