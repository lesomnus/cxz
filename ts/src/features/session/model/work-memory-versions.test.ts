import { create } from "@bufbuild/protobuf";
import { describe, expect, it, vi } from "vitest";
import { MemorySchema } from "#gen/cxz/memory_pb";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { SessionRefSchema } from "#gen/cxz/session_svc_pb";
import { MemoryVersionsReplySchema } from "#gen/cxz/memory_svc_pb";
import { ReadCache } from "#src/shared/api/read-cache.ts";
import type { WorkMemorySource } from "./work-memories";
import {
  initialMemoryVersion,
  loadWorkMemoryVersions,
} from "./work-memory-versions";

const id = (value: number) =>
  Uint8Array.from({ length: 16 }, (_, i) => (i === 15 ? value : 0));
const memory = (value: number, revision = 2n) =>
  create(MemorySchema, { id: id(value), revision });
const source = (
  versions: WorkMemorySource["memories"]["versions"],
): WorkMemorySource => ({
  project: create(ProjectSchema, { id: id(100) }),
  session: create(SessionRefSchema, { key: { case: "id", value: id(101) } }),
  cache: new ReadCache(),
  memories: { versions, watch: async function* () {} },
});

describe("work-menu Memory revision metadata", () => {
  it("requests the displayed revision only and reuses its cached metadata on reopen", async () => {
    const versions = vi.fn<WorkMemorySource["memories"]["versions"]>(
      async (request) =>
        create(MemoryVersionsReplySchema, {
          versions: [
            {
              memoryId: id(1),
              revision: request.beforeRevision! - 1n,
              bytesAdded: 42n,
              bytesRemoved: 3n,
            },
          ],
        }),
    );
    const context = source(versions);
    const rows = [memory(1, 7n), memory(2, 1n)];
    const first = await loadWorkMemoryVersions(
      context,
      rows,
      new AbortController().signal,
    );
    const again = await loadWorkMemoryVersions(
      context,
      rows,
      new AbortController().signal,
    );
    expect(first).toEqual(again);
    expect([...first.values()][0].bytesAdded).toBe(42n);
    expect(versions).toHaveBeenCalledTimes(1);
    expect(versions.mock.calls[0][0]).toMatchObject({
      beforeRevision: 8n,
      size: 1,
      ref: { key: { case: "id", value: id(1) } },
    });
    expect(initialMemoryVersion(rows[1])?.revision).toBe(1n);
    expect(initialMemoryVersion(rows[0])).toBeUndefined();
  });
  it("bounds concurrent requests and keeps missing revisions unknown without blocking other rows", async () => {
    let active = 0,
      maximum = 0;
    const versions = vi.fn<WorkMemorySource["memories"]["versions"]>(
      async (request) => {
        active++;
        maximum = Math.max(maximum, active);
        await new Promise((resolve) => setTimeout(resolve, 1));
        active--;
        if ((request.ref?.key?.value as Uint8Array)[15] === 3)
          throw new Error("revision unavailable");
        return create(MemoryVersionsReplySchema, {
          versions: [{ revision: 2n, bytesAdded: 1n, bytesRemoved: 0n }],
        });
      },
    );
    const result = await loadWorkMemoryVersions(
      source(versions),
      Array.from({ length: 9 }, (_, i) => memory(i)),
      new AbortController().signal,
    );
    expect(maximum).toBeLessThanOrEqual(4);
    expect(result.size).toBe(8);
  });
});
