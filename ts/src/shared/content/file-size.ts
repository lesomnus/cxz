import { isFieldSet } from "@bufbuild/protobuf";
import {
  ProjectDownloadReplySchema,
  type ProjectDownloadReply,
} from "#gen/cxz/project_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { projectRef } from "#src/shared/api/project-id.ts";
import { readBy, release } from "#src/shared/api/stream-deadline.ts";

export type FileSizeReader = (
  path: string,
  signal: AbortSignal,
) => Promise<bigint>;

export function createFileSizeReader(
  projects: Pick<Connection["projects"], "download">,
  project: string,
): FileSizeReader {
  const cache = new Map<string, { size: bigint; expires: number }>();
  return async (path, signal) => {
    signal.throwIfAborted();
    const cached = cache.get(path);
    if (cached && cached.expires > Date.now()) return cached.size;
    const controller = new AbortController();
    const abort = () => controller.abort(signal.reason);
    signal.addEventListener("abort", abort, { once: true });
    let stream: AsyncIterator<ProjectDownloadReply> | undefined;
    try {
      stream = projects
        .download(
          { ref: projectRef(project), path },
          { signal: controller.signal, timeoutMs: 5000 },
        )
        [Symbol.asyncIterator]();
      const reply = await readBy(
        stream.next(),
        5000,
        "File information timed out",
      );
      if (
        reply.done ||
        !isFieldSet(reply.value, ProjectDownloadReplySchema.field.totalSize) ||
        reply.value.totalSize < 0n
      )
        throw new Error("File size unavailable");
      const size = reply.value.totalSize;
      if (!signal.aborted) {
        if (cache.size >= 256) cache.delete(cache.keys().next().value!);
        cache.set(path, { size, expires: Date.now() + 30000 });
      }
      return size;
    } finally {
      // The first reply carries the size. Do not consume the file body.
      controller.abort();
      if (stream) release(stream);
      signal.removeEventListener("abort", abort);
    }
  };
}
