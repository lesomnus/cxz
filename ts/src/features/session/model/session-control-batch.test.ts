import { describe, expect, it, vi } from "vitest";
import { create } from "@bufbuild/protobuf";
import { SessionSchema } from "#gen/cxz/session_pb";
import {
  canControl,
  controlTarget,
  performSessionControl,
  previewSessionPurge,
  settleSessions,
  type SessionControlConnection,
} from "./session-control-batch";

function fixture() {
  const session = create(SessionSchema, {
    id: new Uint8Array([1]),
    runtimeId: "one",
    name: "First",
    clientId: "creation",
    status: { state: "idle", runId: "run-1" },
  });
  const mocks = {
    sessions: {
      get: vi.fn().mockResolvedValue(session),
      purge: vi.fn().mockResolvedValue({ targets: [], retained: [] }),
      stop: vi.fn().mockResolvedValue(session),
      resume: vi.fn().mockResolvedValue(session),
    },
    store: { apply: vi.fn() },
    drafts: { delete: vi.fn() },
    attention: { reset: vi.fn() },
    conversations: { delete: vi.fn() },
  };
  return { session, mocks, c: mocks as unknown as SessionControlConnection };
}

describe("project session control batches", () => {
  it("only stops active runs and only resumes stopped, interrupted or failed runs", () => {
    for (const state of [
      "stopped",
      "interrupted",
      "failed",
      "idle",
      "working",
      "waiting_input",
    ]) {
      const s = create(SessionSchema, { status: { state, runId: "run" } });
      const stopped = ["stopped", "interrupted", "failed"].includes(state);
      expect(canControl(s, "stop")).toBe(!stopped);
      expect(canControl(s, "resume")).toBe(stopped);
      expect(canControl(s, "restart")).toBe(true);
    }
    expect(canControl(create(SessionSchema), "restart")).toBe(false);
    expect(canControl(create(SessionSchema), "purge")).toBe(true);
  });

  it("freezes the run and identity used by a confirmation", () => {
    const { session } = fixture(),
      target = controlTarget(session);
    session.status!.runId = "other-run";
    session.id[0] = 2;
    expect(target.run).toBe("run-1");
    expect(target.id[0]).toBe(1);
  });

  it("previews without cleanup, then cleans caches only after confirmed purge succeeds", async () => {
    const { session, mocks, c } = fixture(),
      target = controlTarget(session);
    await previewSessionPurge(c, target);
    expect(mocks.sessions.purge.mock.calls[0][0].dryRun).toBe(true);
    expect(mocks.store.apply).not.toHaveBeenCalled();
    await performSessionControl(c, target, "purge");
    expect(mocks.sessions.purge.mock.calls[1][0].dryRun).not.toBe(true);
    expect(mocks.store.apply).toHaveBeenCalledWith("cxz.Session", [
      { id: target.id },
    ]);
    expect(mocks.drafts.delete).toHaveBeenCalledWith("one");
    expect(mocks.attention.reset).toHaveBeenCalledWith("one");
    expect(mocks.conversations.delete).toHaveBeenCalledWith("one");
  });

  it("rejects a changed run, identity or creation key before purging", async () => {
    for (const change of ["run", "id", "creation"]) {
      const { session, mocks, c } = fixture(),
        target = controlTarget(session);
      if (change === "run") session.status!.runId = "other-run";
      if (change === "id") session.id[0] = 2;
      if (change === "creation") session.clientId = "other-creation";
      await expect(performSessionControl(c, target, "purge")).rejects.toThrow(
        "Session run changed",
      );
      expect(mocks.sessions.purge).not.toHaveBeenCalled();
      expect(mocks.store.apply).not.toHaveBeenCalled();
    }
  });

  it("does not discard local data after an unsuccessful purge", async () => {
    const { session, mocks, c } = fixture();
    mocks.sessions.purge.mockRejectedValue(new Error("disconnected"));
    await expect(
      performSessionControl(c, controlTarget(session), "purge"),
    ).rejects.toThrow("disconnected");
    expect(mocks.store.apply).not.toHaveBeenCalled();
    expect(mocks.drafts.delete).not.toHaveBeenCalled();
    expect(mocks.conversations.delete).not.toHaveBeenCalled();
  });

  it("bounds concurrency, retains every outcome in order and never automatically retries failures", async () => {
    const { session } = fixture();
    const targets = Array.from({ length: 8 }, (_, i) => ({
      ...controlTarget(session),
      runtimeId: String(i),
    }));
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    let active = 0,
      max = 0;
    const calls: string[] = [];
    const result = settleSessions(targets, async (target) => {
      calls.push(target.runtimeId);
      active++;
      max = Math.max(max, active);
      await gate;
      active--;
      if (target.runtimeId === "1") throw new Error("failed");
      return target.runtimeId;
    });
    expect(calls).toHaveLength(3);
    release();
    const results = await result;
    expect(max).toBeLessThanOrEqual(3);
    expect(calls).toEqual(targets.map((t) => t.runtimeId));
    expect(results.map((r) => r.target.runtimeId)).toEqual(calls);
    expect(results[1]).toMatchObject({ ok: false, error: "Error: failed" });
    expect(results.filter((r) => r.ok)).toHaveLength(7);
  });
});
