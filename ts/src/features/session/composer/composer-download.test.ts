import { create } from "@bufbuild/protobuf";
import { expect, it, vi } from "vitest";
import { ProjectDownloadReplySchema } from "#gen/cxz/project_svc_pb";
import { containerDownload } from "./composer-download";

const clientFor = (size: bigint, chunks: Uint8Array[]) => ({
  download: vi.fn(async function* () {
    yield create(ProjectDownloadReplySchema, { totalSize: size });
    for (const data of chunks)
      yield create(ProjectDownloadReplySchema, { data });
  }),
});

it("downloads chunked binary bytes through the project-scoped API, including empty files", async () => {
  const signal = new AbortController().signal;
  const client = clientFor(4n, [
    new Uint8Array([0, 255]),
    new Uint8Array([13, 10]),
  ]);
  const blob = await containerDownload(
    client,
    "project-2",
    "~/file.bin",
    signal,
  );
  expect([...new Uint8Array(await blob.arrayBuffer())]).toEqual([
    0, 255, 13, 10,
  ]);
  expect(client.download).toHaveBeenCalledWith(
    {
      ref: { key: { case: "alias", value: "project-2" } },
      path: "~/file.bin",
    },
    { signal, timeoutMs: 120000 },
  );
  expect(
    (await containerDownload(clientFor(0n, []), "p", "/empty", signal)).size,
  ).toBe(0);
});

it("does not produce a saved file after truncated, oversized or canceled transfers", async () => {
  const signal = new AbortController().signal;
  for (const size of [1n, 3n])
    await expect(
      containerDownload(
        clientFor(size, [new Uint8Array([1, 2])]),
        "p",
        "/file",
        signal,
      ),
    ).rejects.toThrow("incomplete");
  const controller = new AbortController();
  controller.abort();
  await expect(
    containerDownload(clientFor(0n, []), "p", "/file", controller.signal),
  ).rejects.toThrow();
});
