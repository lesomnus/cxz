import { create } from "@bufbuild/protobuf";
import { expect, it, vi } from "vitest";
import { ProjectDownloadReplySchema } from "#gen/cxz/project_svc_pb";
import { createFileSizeReader } from "./file-size";

it.each([0n, 12345n])(
  "reads only the size header and caches %s bytes",
  async (size) => {
    let closed = false;
    let bodyRead = false;
    let signal: AbortSignal | undefined;
    const download = vi.fn(async function* (_request, options) {
      signal = options?.signal;
      try {
        yield create(ProjectDownloadReplySchema, { totalSize: size });
        bodyRead = true;
        yield create(ProjectDownloadReplySchema, {
          data: new Uint8Array(1024),
        });
      } finally {
        closed = true;
      }
    });
    const read = createFileSizeReader({ download }, "project-one");
    expect(await read("/workspace/file.ts", new AbortController().signal)).toBe(
      size,
    );
    expect(bodyRead).toBe(false);
    expect(signal?.aborted).toBe(true);
    expect(closed).toBe(true);
    expect(await read("/workspace/file.ts", new AbortController().signal)).toBe(
      size,
    );
    expect(download).toHaveBeenCalledTimes(1);
  },
);

it("does not treat a missing size header as an empty file", async () => {
  const download = vi.fn(async function* () {
    yield create(ProjectDownloadReplySchema, { data: new Uint8Array([1]) });
  });
  const read = createFileSizeReader({ download }, "project-one");
  await expect(read("/file", new AbortController().signal)).rejects.toThrow(
    "File size unavailable",
  );
  await expect(read("/file", new AbortController().signal)).rejects.toThrow();
  expect(download).toHaveBeenCalledTimes(2);
});

it("does not start a request after the hover has been canceled", async () => {
  const download = vi.fn(async function* () {
    yield create(ProjectDownloadReplySchema, { totalSize: 1n });
  });
  const controller = new AbortController();
  controller.abort();
  await expect(
    createFileSizeReader({ download }, "project-one")(
      "/file",
      controller.signal,
    ),
  ).rejects.toThrow();
  expect(download).not.toHaveBeenCalled();
});

it("cancels an in-flight lookup when the pointer leaves", async () => {
  let stopped = false;
  const download = vi.fn(async function* (_request, options) {
    const signal = options!.signal!;
    await new Promise((_, reject) =>
      signal.addEventListener(
        "abort",
        () => {
          stopped = true;
          reject(signal.reason);
        },
        { once: true },
      ),
    );
    yield create(ProjectDownloadReplySchema, { totalSize: 1n });
  });
  const controller = new AbortController();
  const pending = createFileSizeReader({ download }, "project-one")(
    "/file",
    controller.signal,
  );
  controller.abort();
  await expect(pending).rejects.toThrow();
  expect(stopped).toBe(true);
});

it("keeps identical paths in different projects separate", async () => {
  const download = vi.fn(async function* (request) {
    yield create(ProjectDownloadReplySchema, {
      totalSize: request.ref.key.value === "project-one" ? 10n : 20n,
    });
  });
  const first = createFileSizeReader({ download }, "project-one");
  const second = createFileSizeReader({ download }, "project-two");
  expect(await first("/file", new AbortController().signal)).toBe(10n);
  expect(await second("/file", new AbortController().signal)).toBe(20n);
  expect(await first("/file", new AbortController().signal)).toBe(10n);
  expect(download).toHaveBeenCalledTimes(2);
});
