import { create } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import { authenticationRequired, authenticationState } from "./session-auth";
import { mergeMetadata } from "./session-metadata";

const event = (
  seq: number,
  kind: string,
  text: string,
  p: unknown,
  runId = "run",
) =>
  create(SessionEventSchema, {
    seq: BigInt(seq),
    kind,
    text,
    runId,
    payload: new TextEncoder().encode(JSON.stringify(p)),
  });
const expired = {
  type: "result",
  subtype: "success",
  is_error: true,
  result:
    "Failed to authenticate: OAuth session expired and could not be refreshed",
};
it("recognizes Claude's failed result even with subtype success, without retaining its native body", () => {
  const native = event(10, "turn_end", "failed", {
    ...expired,
    extra: "x".repeat(100000),
  });
  const metadata = mergeMetadata([], [native]);
  expect(authenticationRequired(metadata, "run")).toBe(true);
  expect(metadata[0].payload.length).toBeLessThan(100);
  const compact = event(10, "turn_end", "", { authentication_required: true });
  expect(authenticationRequired(mergeMetadata([], [compact]), "run")).toBe(
    true,
  );
});
it("does not mistake prose, tool errors, usage, interrupted turns, or retrying errors for authentication failure", () => {
  for (const e of [
    event(1, "assistant", expired.result, { code: 401 }),
    event(2, "tool_result", expired.result, { code: 401 }),
    event(3, "turn_end", "completed", { ...expired, is_error: false }),
    event(4, "turn_end", "interrupted", { code: 401 }),
    event(5, "turn_end", "failed", {
      usage: { tokens: 401 },
      result: "authentication failed",
    }),
    event(6, "diagnostic", "Authentication failed", { willRetry: true }),
  ])
    expect(authenticationState(e)).not.toBe(true);
});
it("clears recovery on newer completion or new run; paging old failures cannot resurrect it", () => {
  const failed = event(10, "turn_end", "failed", expired);
  let metadata = mergeMetadata([], [failed]);
  expect(authenticationRequired(metadata, "new-run")).toBe(false);
  metadata = mergeMetadata(metadata, [event(20, "turn_end", "completed", {})]);
  metadata = mergeMetadata(metadata, [failed]);
  expect(authenticationRequired(metadata, "run")).toBe(false);
  expect(
    authenticationRequired(
      mergeMetadata(
        metadata,
        Array.from({ length: 300 }, (_, i) =>
          event(30 + i, "usage", "context", {}),
        ),
      ),
      "run",
    ),
  ).toBe(false);
});
