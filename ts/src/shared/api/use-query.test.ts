import { create } from "@bufbuild/protobuf";
import { Code } from "@connectrpc/connect";
import { expect, test, vi } from "vitest";
import {
  MemoryReadFormat,
  MemoryService,
  MemoryReadRequestSchema,
} from "#gen/cxz/memory_svc_pb";
import { projectRef, projectId } from "./project-id";
import { Connection } from "./connection";
import { memoryFixture } from "#src/storybook/memory-fixture.ts";
import { refreshQuery } from "./use-query";

function document(f: ReturnType<typeof memoryFixture>) {
  const memory = f.memories[0];
  return {
    memory,
    request: {
      ref: {
        key: { case: "id" as const, value: memory.id },
        project: projectRef(projectId(memory.project)),
      },
      format: MemoryReadFormat.FULL,
    },
  };
}

test("Payday retains full Memory Read responses across remount and revalidates without losing the cached body", async () => {
  const f = memoryFixture({ readDelay: 30 });
  const { memory, request } = document(f);
  const get = () =>
    f.c.queries.get(MemoryService.method.read, request, { watch: false });
  const first = get();
  const off = f.c.queries.subscribe(first.key, () => {});
  await vi.waitFor(() => expect(get().state).toBe("ok"));
  expect(get().data?.content).toBe(memory.content);
  off();
  expect(f.readRequests).toHaveLength(1);
  const cached = get();
  expect(cached.data?.content).toBe(memory.content);
  expect(f.readRequests).toHaveLength(1);
  const back = f.c.queries.subscribe(cached.key, () => {});
  expect(get().data?.content).toBe(memory.content);
  await vi.waitFor(() => expect(f.readRequests).toHaveLength(2));
  back();
  f.c.dispose();
});

test("refresh waits for a complete new body at the confirmed revision, rather than an intermediate metadata notification", async () => {
  const f = memoryFixture();
  const { memory, request } = document(f);
  const get = () =>
    f.c.queries.get(MemoryService.method.read, request, { watch: false });
  const off = f.c.queries.subscribe(get().key, () => {});
  await vi.waitFor(() => expect(get().state).toBe("ok"));
  const saved = await f.c.memories.patch({
    ref: request.ref,
    dateUpdated: memory.dateUpdated,
    baseRevision: memory.revision,
    content: "Updated content",
  });
  const result = await refreshQuery(
    f.c.queries,
    MemoryService.method.read,
    request,
    (result) => result.revision >= saved.revision,
  );
  expect(result.content).toBe("Updated content");
  expect(result.revision).toBe(saved.revision);
  off();
  expect(get().data?.content).toBe("Updated content");
  f.c.dispose();
});

test("query keys isolate project scopes and authenticated connections", async () => {
  const f = memoryFixture();
  const { request } = document(f);
  const first = f.c.queries.get(MemoryService.method.read, request, {
    watch: false,
  });
  const other = f.c.queries.get(
    MemoryService.method.read,
    {
      ...request,
      ref: { ...request.ref, project: projectRef(projectId(f.projects[1])) },
    },
    { watch: false },
  );
  expect(first.key).not.toBe(other.key);
  const next = memoryFixture();
  expect(
    next.c.queries.get(MemoryService.method.read, document(next).request, {
      watch: false,
    }).data,
  ).toBeUndefined();
  const bound = new Connection(
    "http://storybook",
    f.c.transport,
    undefined,
    "authenticated-reader",
  );
  expect(bound.store.identity).toBe("authenticated-reader");
  f.c.dispose();
  next.c.dispose();
  bound.dispose();
});

test("a failed or canceled refresh settles instead of leaving save pending", async () => {
  const f = memoryFixture();
  const { request } = document(f);
  const get = () =>
    f.c.queries.get(MemoryService.method.read, request, { watch: false });
  const off = f.c.queries.subscribe(get().key, () => {});
  await vi.waitFor(() => expect(get().state).toBe("ok"));
  const cancel = new AbortController();
  const pending = refreshQuery(
    f.c.queries,
    MemoryService.method.read,
    request,
    () => false,
    cancel.signal,
  );
  cancel.abort();
  await expect(pending).rejects.toMatchObject({ name: "AbortError" });
  const wrong = create(MemoryReadRequestSchema, {
    ...request,
    ref: { ...request.ref, project: projectRef(projectId(f.projects[1])) },
  });
  await expect(
    refreshQuery(f.c.queries, MemoryService.method.read, wrong),
  ).rejects.toMatchObject({ code: Code.NotFound });
  off();
  f.c.dispose();
});

test("a refresh that never reaches the requested revision times out", async () => {
  const f = memoryFixture();
  const { request } = document(f);
  const get = () =>
    f.c.queries.get(MemoryService.method.read, request, { watch: false });
  const off = f.c.queries.subscribe(get().key, () => {});
  await vi.waitFor(() => expect(get().state).toBe("ok"));
  vi.useFakeTimers();
  try {
    const pending = refreshQuery(
      f.c.queries,
      MemoryService.method.read,
      request,
      () => false,
    );
    const failure = expect(pending).rejects.toThrow("refresh timed out");
    await vi.advanceTimersByTimeAsync(30000);
    await failure;
  } finally {
    vi.useRealTimers();
    off();
    f.c.dispose();
  }
});
