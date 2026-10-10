import { create } from "@bufbuild/protobuf";
import { describe, expect, it, vi } from "vitest";
import { SessionSchema } from "../gen/cxz/session_pb";
import { editSession } from "./session-edit";

function client() {
  return {
    auxRun: vi.fn().mockResolvedValue({ title: "New title" }),
    patch: vi.fn().mockResolvedValue(create(SessionSchema)),
    get: vi.fn().mockResolvedValue(
      create(SessionSchema, {
        runtimeId: "stable-id",
        name: "New title",
        alias: "oak",
      }),
    ),
  };
}
describe("session identity editing", () => {
  it("patches a manual title without invoking auxiliary AI", async () => {
    const api = client();
    const saved = await editSession(api, "stable-id", "name", "New title");
    expect(api.patch.mock.calls[0][0]).toEqual({
      ref: { key: { case: "runtimeId", value: "stable-id" } },
      name: "New title",
    });
    expect(api.auxRun).not.toHaveBeenCalled();
    expect(api.get.mock.invocationCallOrder[0]).toBeGreaterThan(
      api.patch.mock.invocationCallOrder[0],
    );
    expect(saved.name).toBe("New title");
  });
  it("renames by stable runtime ID, then refreshes the full session and project", async () => {
    const api = client();
    await editSession(api, "stable-id", "alias", "oak-tree");
    expect(api.patch.mock.calls[0][0]).toEqual({
      ref: { key: { case: "runtimeId", value: "stable-id" } },
      alias: "oak-tree",
    });
    expect(api.auxRun).not.toHaveBeenCalled();
    expect(api.get.mock.calls[0][0].select).toEqual({
      all: true,
      project: { all: true },
    });
  });
  it("does not refresh or report success after a duplicate-alias rejection", async () => {
    const api = client();
    api.patch.mockRejectedValue(new Error("Alias already in use"));
    await expect(editSession(api, "stable-id", "alias", "oak")).rejects.toThrow(
      "Alias already in use",
    );
    expect(api.get).not.toHaveBeenCalled();
  });
  it("refuses invalid fields before making a request", async () => {
    const api = client();
    for (const alias of [
      "ab",
      "Bad",
      "oak--tree",
      "oak-",
      "oak_tree",
      "a".repeat(21),
    ])
      await expect(
        editSession(api, "stable-id", "alias", alias),
      ).rejects.toThrow("Alias must be");
    await expect(editSession(api, "stable-id", "name", "")).rejects.toThrow(
      "Enter a session title",
    );
    expect(api.auxRun).not.toHaveBeenCalled();
    expect(api.patch).not.toHaveBeenCalled();
  });
});
