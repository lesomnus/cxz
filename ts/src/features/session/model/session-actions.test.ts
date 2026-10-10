import { describe, expect, it, vi } from "vitest";
import { create } from "@bufbuild/protobuf";
import { SessionSchema } from "../../../../gen/cxz/session_pb";
import { SessionRefSchema } from "../../../../gen/cxz/session_svc_pb";
import { manageSession } from "./session-actions";

const ref = create(SessionRefSchema, {
  key: { case: "runtimeId", value: "session" },
});
const session = (runId = "run-1", state = "idle") =>
  create(SessionSchema, { runtimeId: "session", status: { runId, state } });
function client() {
  return {
    get: vi.fn().mockResolvedValue(session()),
    stop: vi.fn().mockResolvedValue(session("run-1", "stopped")),
    resume: vi.fn().mockResolvedValue(session("run-2")),
  };
}
describe("session lifecycle controls", () => {
  it("restarts through Stop, verifies the run again, then resumes with a new command identity", async () => {
    const api = client();
    await expect(
      manageSession(api, ref, "restart", "run-1"),
    ).resolves.toMatchObject({ status: { runId: "run-2" } });
    expect(api.get).toHaveBeenCalledTimes(2);
    expect(api.stop.mock.invocationCallOrder[0]).toBeLessThan(
      api.get.mock.invocationCallOrder[1],
    );
    expect(api.get.mock.invocationCallOrder[1]).toBeLessThan(
      api.resume.mock.invocationCallOrder[0],
    );
    expect(api.stop.mock.calls[0][0].clientId).not.toBe(
      api.resume.mock.calls[0][0].clientId,
    );
    expect(api.resume.mock.calls[0][0].runId).toBe("run-1");
  });
  it("resumes a stopped session without stopping it again", async () => {
    const api = client();
    api.get.mockResolvedValue(session("run-1", "stopped"));
    await manageSession(api, ref, "restart", "run-1");
    expect(api.stop).not.toHaveBeenCalled();
    expect(api.resume).toHaveBeenCalledOnce();
  });
  it("does not resume if Stop fails", async () => {
    const api = client();
    api.stop.mockRejectedValue(new Error("Stop failed"));
    await expect(manageSession(api, ref, "restart", "run-1")).rejects.toThrow(
      "Stop failed",
    );
    expect(api.resume).not.toHaveBeenCalled();
  });
  it("never adopts another client's run after stopping", async () => {
    const api = client();
    api.get
      .mockResolvedValueOnce(session())
      .mockResolvedValueOnce(session("other-run"));
    await expect(manageSession(api, ref, "restart", "run-1")).rejects.toThrow(
      "Session run changed",
    );
    expect(api.resume).not.toHaveBeenCalled();
  });
  it("rejects stale menu actions before stopping or resuming", async () => {
    const api = client();
    api.get.mockResolvedValue(session("other-run"));
    await expect(manageSession(api, ref, "stop", "run-1")).rejects.toThrow(
      "Session run changed",
    );
    expect(api.stop).not.toHaveBeenCalled();
    expect(api.resume).not.toHaveBeenCalled();
  });
});
