import { create, toBinary } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import { eventSource, eventListSource } from "./event-source";
import { detail, payload } from "./journal";

it("exposes stable readers without exposing protocol buffers as enumerable props", () => {
  const event = create(SessionEventSchema, {
    seq: 1n,
    payload: new TextEncoder().encode(
      JSON.stringify({ input: "x".repeat(600000) }),
    ),
  });
  const wire = toBinary(SessionEventSchema, event);
  const rows = [event];
  const source = eventSource(event);
  const list = eventListSource(rows);
  expect(eventSource(event)).toBe(source);
  expect(eventListSource(rows)).toBe(list);
  expect(source()).toBe(event);
  expect(list()).toBe(rows);
  expect(Object.keys(source)).toEqual([]);
  expect(Object.keys(list)).toEqual([]);
  expect(eventListSource([...rows])).not.toBe(list);
  expect(toBinary(SessionEventSchema, source())).toEqual(wire);
  expect(JSON.parse(detail(source())).input).toHaveLength(600000);
});

it("reuses decoded payload snapshots and invalidates when the buffer changes", () => {
  const event = create(SessionEventSchema, {
    payload: new TextEncoder().encode('{"model":"first"}'),
  });
  const first = payload(event);
  expect(payload(event)).toBe(first);
  expect(payload({ ...event })).toBe(first);
  const replaced = {
    ...event,
    payload: new TextEncoder().encode('{"model":"second"}'),
  };
  expect(payload(replaced)).not.toBe(first);
  expect(payload(replaced).model).toBe("second");
  expect(first.model).toBe("first");
});
