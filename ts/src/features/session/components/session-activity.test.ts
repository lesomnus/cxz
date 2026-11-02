import { create } from "@bufbuild/protobuf";
import { expect, test } from "vitest";
import { SessionSchema } from "#gen/cxz/session_pb";
import { activityDots, sessionActivity } from "./session-activity";

function session(id = 1, runId = "run-1") {
  return create(SessionSchema, {
    id: Uint8Array.from({ length: 16 }, (_, index) => (index === 15 ? id : 0)),
    runtimeId: `session-${id}`,
    status: { state: "working", runId },
  });
}

test("fresh catalog objects reproduce the same phase and palette after reload", () => {
  expect(sessionActivity(session())).toEqual(sessionActivity(session()));
});

test("sessions in one run receive distinct phases instead of starting together", () => {
  const appearances = Array.from({ length: 12 }, (_, index) =>
    sessionActivity(session(index + 1)),
  );
  expect(new Set(appearances.map(({ phase }) => phase)).size).toBe(
    appearances.length,
  );
  expect(
    new Set(appearances.map(({ colors }) => colors.join(","))).size,
  ).toBeGreaterThan(1);
});

test("a new run changes the appearance while a recurring catalog snapshot keeps it", () => {
  expect(sessionActivity(session(1, "run-2"))).not.toEqual(
    sessionActivity(session(1, "run-1")),
  );
  expect(sessionActivity(session(1, "run-2"))).toEqual(
    sessionActivity(session(1, "run-2")),
  );
});

test("renames and live status updates do not reseed an existing run", () => {
  const original = session();
  const changed = session();
  changed.alias = "renamed";
  changed.name = "New title";
  changed.status!.lastSeq = 100n;
  changed.status!.state = "waiting_input";
  expect(sessionActivity(changed)).toEqual(sessionActivity(original));
});

test("all palette colors remain present and phases stay inside one cycle", () => {
  for (let id = 0; id < 32; id++) {
    const { phase, colors } = sessionActivity(session(id));
    expect(phase).toBeGreaterThanOrEqual(0);
    expect(phase).toBeLessThan(activityDots.length);
    expect([...colors].sort()).toEqual([
      "cyan",
      "cyan",
      "cyan",
      "green",
      "green",
      "green",
      "violet",
      "violet",
    ]);
  }
});

test("legacy rows without a UUID use their runtime identity", () => {
  const first = session(1),
    second = session(2);
  first.id = second.id = new Uint8Array();
  expect(sessionActivity(first)).toEqual(
    sessionActivity({ ...first, id: new Uint8Array() }),
  );
  expect(sessionActivity(first)).not.toEqual(sessionActivity(second));
});
