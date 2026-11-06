import { create } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import { ContainerState, ProjectSchema } from "#gen/cxz/project_pb";
import { canControlProject, settleProjects } from "./project-control-batch";

it("filters mixed selections by the same lifecycle rules as row controls", () => {
  for (const [state, expected] of [
    [ContainerState.RUNNING, [true, false, true, true]],
    [ContainerState.CREATED, [false, true, true, true]],
    [ContainerState.STOPPED, [false, true, true, true]],
    [ContainerState.ABSENT, [false, true, false, true]],
    [ContainerState.UNSPECIFIED, [false, false, false, true]],
  ] as const) {
    const project = create(ProjectSchema, {
      status: { containerState: state },
    });
    expect(
      (["stop", "start", "remove", "erase"] as const).map((kind) =>
        canControlProject(project, kind),
      ),
    ).toEqual(expected);
  }
});

it("continues after individual failures, preserves structured errors and order, and bounds concurrency", async () => {
  const projects = Array.from({ length: 8 }, (_, i) =>
    create(ProjectSchema, { alias: `p-${i}` }),
  );
  const error = new Error("needs trust");
  let running = 0,
    peak = 0;
  const results = await settleProjects(projects, async (p) => {
    peak = Math.max(peak, ++running);
    await new Promise((resolve) => setTimeout(resolve, 1));
    running--;
    if (p === projects[1]) throw error;
  });
  expect(peak).toBe(3);
  expect(results.map((r) => r.project)).toEqual(projects);
  expect(results.filter((r) => r.ok)).toHaveLength(7);
  expect(results[1]).toEqual({ project: projects[1], ok: false, error });
});
