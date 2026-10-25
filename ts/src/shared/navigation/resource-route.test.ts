import { expect, test } from "vitest";
import { parse } from "@lesomnus/payday/pdid";
import {
  matchesResource,
  resourceHandle,
  resourceRef,
  sessionLocation,
  projectLocation,
  taskLocation,
} from "./resource-route";
import { projectRef } from "#src/shared/api/project-id.ts";
import { taskLookup } from "#src/features/tasks/task-model.ts";
import { create } from "@bufbuild/protobuf";
import { ProjectSchema } from "#gen/cxz/project_pb";

const id = "00000000-0000-8000-8007-000000000001";
const resource = { id: parse(id).bytes, alias: "oak", runtimeId: "old-handle" };

test("resource addresses prefer aliases with UUID fallback and explicit self scope", () => {
  expect(resourceHandle(resource)).toBe("oak");
  expect(resourceHandle({ ...resource, alias: "" })).toBe(id);
  expect(sessionLocation(resource)).toEqual({
    to: "/c/$connection/session/$sessionId",
    params: { connection: "_", sessionId: "oak" },
  });
  expect(projectLocation(id)).toMatchObject({
    params: { connection: "_", projectId: id },
  });
  expect(
    taskLocation({ project: "cxz", task: "fix-login", run: "run-1" }),
  ).toMatchObject({
    params: { connection: "_", taskId: "fix-login" },
    search: { project: "cxz", run: "run-1" },
  });
});

test("UUIDs and aliases use distinct API reference keys", () => {
  expect(resourceRef(id).key).toEqual({ case: "id", value: resource.id });
  expect(resourceRef("oak").key).toEqual({ case: "alias", value: "oak" });
  expect(resourceRef("550e8400-e29b-41d4-a716-446655440000").key.case).toBe(
    "id",
  );
  expect(projectRef("cxz").key).toEqual({ case: "alias", value: "cxz" });
  expect(projectRef("0123456789abcdef01234567").key.case).toBe("runtimeId");
  const project = create(ProjectSchema, { id: resource.id });
  expect(taskLookup(project, id).key).toEqual({
    case: "id",
    value: resource.id,
  });
});

test("aliases, UUIDs and legacy runtime handles resolve to the same resource without confusing a UUID for an alias", () => {
  for (const handle of ["oak", id, "old-handle"])
    expect(matchesResource(resource, handle)).toBe(true);
  expect(matchesResource(resource, "different")).toBe(false);
  expect(
    matchesResource({ ...resource, id: new Uint8Array(16), alias: id }, id),
  ).toBe(false);
});

test("route queries preserve large sequence numbers and encoded references verbatim", async () => {
  const { parseRouteSearch, stringifyRouteSearch } = await import(
    "./resource-route"
  );
  const search = {
    seq: "18446744073709551615",
    project: "project-one",
    q: "a+b & c",
    absent: undefined,
  };
  expect(parseRouteSearch(stringifyRouteSearch(search))).toEqual({
    seq: search.seq,
    project: search.project,
    q: search.q,
  });
});

test("old JSON-quoted sequence links retain the exact sequence value", async () => {
  const { parseRouteSearch } = await import("./resource-route");
  expect(parseRouteSearch("?seq=%222%22").seq).toBe("2");
  expect(parseRouteSearch("?seq=%2218446744073709551615%22").seq).toBe(
    "18446744073709551615",
  );
});
