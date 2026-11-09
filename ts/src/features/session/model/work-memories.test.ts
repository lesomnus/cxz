import { create } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";
import { describe, expect, it, vi } from "vitest";
import { Code, ConnectError } from "@connectrpc/connect";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { SessionRefSchema } from "#gen/cxz/session_svc_pb";
import { MemorySchema, type Memory } from "#gen/cxz/memory_pb";
import { MemoryScope, MemoryWatchResponseSchema } from "#gen/cxz/memory_svc_pb";
import { watchWorkMemories } from "./work-memories";

type WatchSource = Parameters<typeof watchWorkMemories>[0];

const id = (value: number) =>
  Uint8Array.from({ length: 16 }, (_, i) => (i === 15 ? value : 0));
const project = create(ProjectSchema, { id: id(1) });
const session = create(SessionRefSchema, { key: { case: "id", value: id(2) } });
const note = (value: number, name: string) =>
  create(MemorySchema, { id: id(value), name });
const frame = (...values: Memory[]) =>
  create(MemoryWatchResponseSchema, {
    items: values.map((value) => ({ id: value.id, value })),
  });

describe("conversation Memory metadata subscription", () => {
  it("uses one session-scoped metadata stream and applies edits and both deletion forms", async () => {
    const stop = new AbortController();
    const snapshots: string[][] = [];
    const second = note(4, "second");
    const watch = vi.fn<WatchSource["memories"]["watch"]>(async function* () {
      yield frame(note(3, "first"));
      yield frame(note(3, "renamed"), second);
      yield create(MemoryWatchResponseSchema, { items: [{ id: id(3) }] });
      yield frame(
        create(MemorySchema, {
          ...second,
          dateDeleted: create(TimestampSchema, { seconds: 1n }),
        }),
      );
    });
    await watchWorkMemories(
      { project, session, memories: { watch } },
      stop.signal,
      (rows) => {
        snapshots.push(rows.map((row) => row.name));
        if (snapshots.length === 4) stop.abort();
      },
    );
    expect(snapshots).toEqual([
      ["first"],
      ["renamed", "second"],
      ["second"],
      [],
    ]);
    expect(watch).toHaveBeenCalledTimes(1);
    expect(watch.mock.calls[0][0]).toEqual({
      project: { key: { case: "id", value: project.id } },
      session,
      scope: MemoryScope.ACTIVE,
    });
    expect(watch.mock.calls[0][1]).toEqual({
      signal: stop.signal,
      timeoutMs: 0,
    });
  });

  it("ignores an old session's delayed snapshot after cancellation", async () => {
    const stop = new AbortController();
    const receive = vi.fn();
    let resolve!: () => void;
    let released = false;
    const ready = new Promise<void>((done) => {
      resolve = done;
    });
    const source: WatchSource = {
      project,
      session,
      memories: {
        watch: async function* () {
          try {
            await ready;
            yield frame(note(3, "late"));
          } finally {
            released = true;
          }
        },
      },
    };
    const pending = watchWorkMemories(source, stop.signal, receive);
    stop.abort();
    resolve();
    await pending;
    expect(receive).not.toHaveBeenCalled();
    expect(released).toBe(true);
  });

  it("reports a disconnected subscription after delivering its last metadata", async () => {
    const source: WatchSource = {
      project,
      session,
      memories: {
        watch: async function* () {
          yield frame(note(3, "retained"));
        },
      },
    };
    const receive = vi.fn();
    await expect(
      watchWorkMemories(source, new AbortController().signal, receive),
    ).rejects.toEqual(
      new ConnectError("Memory subscription disconnected.", Code.Unavailable),
    );
    expect(receive).toHaveBeenCalledOnce();
    expect(receive.mock.calls[0][0][0].name).toBe("retained");
  });
});
