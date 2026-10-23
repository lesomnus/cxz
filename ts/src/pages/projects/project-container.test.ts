import { ContainerState } from "#gen/cxz/project_pb";
import { create } from "@bufbuild/protobuf";
import { describe, expect, it, vi } from "vitest";
import { ProjectSchema } from "#gen/cxz/project_pb";
import {
  removeContainer,
  startContainer,
  stopContainer,
} from "./project-container";

function fixture() {
  const project = create(ProjectSchema, {
    id: new Uint8Array(16).fill(1),
    runtimeId: "project",
    alias: "web",
    status: { containerState: ContainerState.RUNNING },
  });
  const answered = create(ProjectSchema, {
    ...project,
    status: {
      ...project.status!,
      containerState: ContainerState.STOPPED,
    },
  });
  const c = {
    projects: {
      stop: vi.fn().mockResolvedValue(answered),
      start: vi.fn().mockResolvedValue(answered),
      remove: vi.fn().mockResolvedValue(answered),
      erase: vi.fn(),
    },
    store: { apply: vi.fn() },
    inventory: { refresh: vi.fn() },
  };
  return {
    project,
    answered,
    c,
    connection: c as unknown as Parameters<typeof stopContainer>[0],
  };
}

describe("project container controls", () => {
  // One function per RPC: a button that says Stop calls StopContainer, and
  // nothing in here archives or deletes anything.
  const controls = [
    { name: "stop", run: stopContainer, rpc: "stop" },
    { name: "start", run: startContainer, rpc: "start" },
    { name: "remove", run: removeContainer, rpc: "remove" },
  ] as const;
  for (const control of controls) {
    it(`${control.name} calls ${control.rpc} by UUID and keeps the project`, async () => {
      const f = fixture();
      const result = await control.run(f.connection, f.project);
      expect(f.c.projects[control.rpc]).toHaveBeenCalledExactlyOnceWith(
        {
          ref: { key: { case: "id", value: f.project.id } },
          clientId: expect.any(String),
        },
        { timeoutMs: 120_000 },
      );
      for (const other of controls) {
        if (other.rpc !== control.rpc)
          expect(f.c.projects[other.rpc]).not.toHaveBeenCalled();
      }
      expect(f.c.projects.erase).not.toHaveBeenCalled();
      expect(result.alias).toBe("web");
      expect(f.c.store.apply).toHaveBeenCalledWith("cxz.Project", [
        { id: f.answered.id, value: f.answered },
      ]);
      expect(f.c.inventory.refresh).toHaveBeenCalledOnce();
    });
  }
  it("preserves local state on failure and does not retry", async () => {
    const f = fixture();
    f.c.projects.stop.mockRejectedValue(new Error("Stop failed"));
    await expect(stopContainer(f.connection, f.project)).rejects.toThrow(
      "Stop failed",
    );
    expect(f.c.projects.stop).toHaveBeenCalledOnce();
    expect(f.c.store.apply).not.toHaveBeenCalled();
    expect(f.c.inventory.refresh).not.toHaveBeenCalled();
  });
});
