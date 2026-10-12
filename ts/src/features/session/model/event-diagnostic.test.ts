import { create } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import { eventDiagnostic } from "./event-diagnostic";

const event = (kind: string, text: string, data: unknown = {}) =>
  create(SessionEventSchema, {
    kind,
    text,
    payload: new TextEncoder().encode(JSON.stringify(data)),
  });

it("reads the recorded provider message and retry decision from a Codex overload", () => {
  expect(
    eventDiagnostic(
      event("diagnostic", "Codex error", {
        error: {
          message:
            "Selected model is at capacity. Please try a different model.",
          codexErrorInfo: "serverOverloaded",
        },
        willRetry: false,
      }),
    ),
  ).toEqual({
    message: "Selected model is at capacity. Please try a different model.",
    code: "serverOverloaded",
    retrying: false,
  });
});

it("keeps legacy plain diagnostics and stderr without inventing a retry decision", () => {
  for (const kind of ["diagnostic", "stderr", "error"])
    expect(
      eventDiagnostic(
        event(kind, "Connection interrupted", { error: { message: 123 } }),
      ),
    ).toEqual({
      message: "Connection interrupted",
      code: undefined,
      retrying: undefined,
    });
  expect(
    eventDiagnostic(event("diagnostic", "Failed", { message: "Token expired" }))
      ?.message,
  ).toBe("Token expired");
  expect(
    eventDiagnostic(event("error", "Failed", { error: "Not authorized" }))
      ?.message,
  ).toBe("Not authorized");
});

it("does not classify successful responses, quota telemetry or user interruptions as errors", () => {
  for (const kind of ["assistant", "input", "tool_output", "usage_status"])
    expect(
      eventDiagnostic(
        event(kind, "An error occurred", { error: { message: "Example" } }),
      ),
    ).toBeUndefined();
  expect(eventDiagnostic(event("turn_end", "interrupted"))).toBeUndefined();
  expect(
    eventDiagnostic(
      event("turn_end", "failed", { error: { message: "Failed turn" } }),
    )?.message,
  ).toBe("Failed turn");
});
