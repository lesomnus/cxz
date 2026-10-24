import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { describe, expect, it, vi } from "vitest";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { editProject, validateProjectField } from "./project-edit";

const project = () =>
  create(ProjectSchema, {
    id: new Uint8Array(16).fill(1),
    alias: "web",
    name: "Workspace",
    desc: "Original description",
    dateUpdated: timestampFromDate(new Date("2026-10-20T00:00:00Z")),
  });
const client = () => ({
  get: vi.fn().mockResolvedValue(project()),
  patch: vi.fn().mockResolvedValue(project()),
});
describe("project metadata editing", () => {
  it.each([
    ["name", "New workspace"],
    ["alias", "  New-Alias  "],
    ["desc", ""],
  ] as const)(
    "patches only %s with the displayed version and stable UUID",
    async (field, value) => {
      const api = client(),
        original = project();
      await editProject(api, original, field, value);
      expect(api.patch.mock.calls[0]![0]).toEqual({
        ref: { key: { case: "id", value: original.id } },
        dateUpdated: original.dateUpdated,
        [field]: field === "alias" ? "new-alias" : value,
      });
      expect(api.get).not.toHaveBeenCalled();
    },
  );
  it("fetches a missing version but never forces an unversioned update", async () => {
    const api = client(),
      original = project();
    original.dateUpdated = undefined;
    await editProject(api, original, "name", "New workspace");
    expect(api.get).toHaveBeenCalledWith(
      {
        ref: { key: { case: "id", value: original.id } },
        select: { all: true },
      },
      expect.anything(),
    );
    api.patch.mockClear();
    api.get.mockResolvedValue(original);
    await expect(editProject(api, original, "desc", "Details")).rejects.toThrow(
      "version is unavailable",
    );
    expect(api.patch).not.toHaveBeenCalled();
  });
  it("propagates conflicts without retrying or overwriting newer fields", async () => {
    const api = client();
    api.patch.mockRejectedValue(new Error("Project changed"));
    await expect(editProject(api, project(), "name", "New")).rejects.toThrow(
      "Project changed",
    );
    expect(api.patch).toHaveBeenCalledTimes(1);
    expect(api.get).not.toHaveBeenCalled();
  });
  it("uses project alias rules and UTF-8 byte limits for names", async () => {
    expect(validateProjectField("alias", "a")).toBeUndefined();
    expect(validateProjectField("alias", "a".repeat(63))).toBeUndefined();
    expect(validateProjectField("name", "가".repeat(66))).toBeUndefined();
    const api = client();
    for (const [field, value] of [
      ["name", " "],
      ["name", "x\ny"],
      ["name", "가".repeat(67)],
      ["alias", "bad_alias"],
      ["alias", "a".repeat(64)],
    ] as const)
      await expect(editProject(api, project(), field, value)).rejects.toThrow();
    expect(api.patch).not.toHaveBeenCalled();
  });
});
