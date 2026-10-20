import { create } from "@bufbuild/protobuf";
import { createClient, createRouterTransport } from "@connectrpc/connect";
import { describe, expect, it, vi } from "vitest";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import { purgeProject } from "./project-purge";

function fixture(mode = "") {
  const project = create(ProjectSchema, {
    id: new Uint8Array(16).fill(1),
    runtimeId: "target",
  });
  const other = create(ProjectSchema, {
    id: new Uint8Array(16).fill(2),
    runtimeId: "other",
  });
  const sessions = [project, project, other].map((p, i) =>
    create(SessionSchema, {
      id: new Uint8Array(16).fill(i + 3),
      project: p,
      runtimeId: `session-${i}`,
    }),
  );
  const calls: boolean[] = [];
  const projects = createClient(
    ProjectService,
    createRouterTransport((router) =>
      router.service(ProjectService, {
        remove(request) {
          calls.push(request.confirmed);
          expect(request.target).toBe("target");
          if (mode === "failure" && request.confirmed)
            throw new Error("Removal failed");
          return {
            project:
              mode === "replaced"
                ? create(ProjectSchema, { ...project, id: other.id })
                : project,
            removed: request.confirmed && mode !== "unconfirmed",
          };
        },
      }),
    ),
  );
  const c = {
    projects,
    store: { apply: vi.fn() },
    drafts: new Map(sessions.map((s) => [s.runtimeId, "draft"])),
    attention: { reset: vi.fn() },
    conversations: { delete: vi.fn() },
    inventory: { snapshot: () => ({ sessions }), refresh: vi.fn() },
  };
  const connection = c as unknown as Parameters<typeof purgeProject>[0];
  return { c, connection, project, sessions, calls };
}
describe("project purge", () => {
  it("verifies the preview then confirms once and clears only the selected project's caches", async () => {
    const f = fixture();
    await purgeProject(f.connection, f.project, [f.sessions[0]!]);
    expect(f.calls).toEqual([false, true]);
    expect(f.c.store.apply).toHaveBeenCalledWith("cxz.Project", [
      { id: f.project.id },
    ]);
    expect(f.c.store.apply).toHaveBeenCalledWith(
      "cxz.Session",
      f.sessions.slice(0, 2).map((s) => ({ id: s.id })),
    );
    expect([...f.c.drafts.keys()]).toEqual(["session-2"]);
    expect(f.c.conversations.delete.mock.calls).toEqual([
      ["session-0"],
      ["session-1"],
    ]);
    expect(f.c.attention.reset.mock.calls).toEqual([
      ["session-0"],
      ["session-1"],
    ]);
    expect(f.c.inventory.refresh).toHaveBeenCalledOnce();
  });
  it("refuses to confirm a target whose resource identity changed", async () => {
    const f = fixture("replaced");
    await expect(
      purgeProject(f.connection, f.project, f.sessions),
    ).rejects.toThrow("Project changed");
    expect(f.calls).toEqual([false]);
    expect(f.c.store.apply).not.toHaveBeenCalled();
  });
  it.each(["failure", "unconfirmed"])(
    "retains local data after %s and never retries",
    async (mode) => {
      const f = fixture(mode);
      await expect(
        purgeProject(f.connection, f.project, f.sessions),
      ).rejects.toThrow();
      expect(f.calls).toEqual([false, true]);
      expect(f.c.store.apply).not.toHaveBeenCalled();
      expect(f.c.drafts.size).toBe(3);
      expect(f.c.inventory.refresh).not.toHaveBeenCalled();
    },
  );
});
