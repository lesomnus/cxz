import type { Connection } from "#src/shared/api/connection.ts";
import { createClient } from "@connectrpc/connect";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import { projectRef } from "#src/shared/api/project-id.ts";
import type { ComposerPaste } from "./composer-pastes";
import type { UploadProgress } from "./composer-upload";

export type SecretFiles = {
  session: string;
  run: string;
  put: (
    bytes: Uint8Array,
    signal: AbortSignal,
    progress?: UploadProgress,
  ) => Promise<string>;
  read?: (path: string, signal: AbortSignal) => Promise<Uint8Array>;
  remove: (path: string) => Promise<void>;
};

export function secretFiles(
  c: Connection,
  project: string,
  session: string,
  run: string,
): SecretFiles {
  return {
    session,
    run,
    put: async (bytes, signal, progress) => {
      const client = progress
        ? createClient(ProjectService, c.uploadTransport(progress))
        : c.projects;
      const result = await client.putSecretFile(
        {
          ref: projectRef(project),
          session: `${session}/${run}`,
          secret: bytes,
        },
        { signal, timeoutMs: 30000 },
      );
      if (!result.path?.startsWith("/"))
        throw new Error("Secret path unavailable");
      return result.path;
    },
    read: (path, signal) => readSecretFile(c.projects, project, path, signal),
    remove: async (path) => {
      await c.projects.deleteSecretFile(
        { ref: projectRef(project), path },
        { timeoutMs: 5000 },
      );
    },
  };
}

// The returned chip is safe to persist immediately. Raw bytes live only in the
// preparation closure and are cleared on completion, error or cancellation.
export function prepareSecretPaste(
  text: string,
  kind: string,
  files?: SecretFiles,
) {
  const paste: ComposerPaste = {
    token: `[Secret ${crypto.randomUUID().replaceAll("-", "").slice(0, 8)}]`,
    body: "",
    bytes: 0,
    lines: 0,
    secret: {
      session: files?.session ?? "",
      run: files?.run ?? "",
      path: "",
      kind,
      state: "uploading",
    },
  };
  const secret = paste.secret!;
  const bytes = new TextEncoder().encode(text);
  async function save(signal: AbortSignal, changed?: () => void) {
    let path = "";
    try {
      signal.throwIfAborted();
      if (bytes.length > 65536) throw new Error("Secret limit: 64 KiB");
      if (!files) throw new Error("Secret paste requires an active session.");
      path = await files.put(bytes, signal, (loaded, total) => {
        if (signal.aborted || secret.state !== "uploading") return;
        secret.progress =
          total > 0
            ? Math.max(0, Math.min(99, Math.floor((loaded / total) * 100)))
            : 0;
        changed?.();
      });
      signal.throwIfAborted();
      secret.path = path;
      secret.state = "ready";
      secret.progress = 100;
      paste.body = `(secret placed at ${path})`;
    } catch {
      // Server errors can include request data. Keep them out of the draft,
      // chip and diagnostics rather than falling back to an ordinary upload.
      secret.state = "error";
      secret.error =
        bytes.length > 65536
          ? "Secret limit: 64 KiB"
          : !files
            ? "Secret paste requires an active session."
            : "Secret preparation failed. Remove this chip and paste again.";
      if (path) await files?.remove(path).catch(() => {});
    } finally {
      bytes.fill(0);
    }
  }
  return { paste, save };
}

export async function readSecretFile(
  client: Pick<Connection["projects"], "download">,
  project: string,
  path: string,
  signal: AbortSignal,
) {
  const bytes = new Uint8Array(65536);
  let size = 0;
  let expected: bigint | undefined;
  try {
    signal.throwIfAborted();
    for await (const reply of client.download(
      { ref: projectRef(project), path },
      { signal, timeoutMs: 30000 },
    )) {
      if (
        reply.totalSize > 0n ||
        (expected === undefined && !reply.data.length)
      )
        expected = reply.totalSize;
      try {
        if (
          reply.totalSize < 0n ||
          reply.totalSize > 65536n ||
          size + reply.data.length > bytes.length
        )
          throw new Error("Secret limit: 64 KiB");
        bytes.set(reply.data, size);
        size += reply.data.length;
      } finally {
        reply.data.fill(0);
      }
    }
    signal.throwIfAborted();
    if (expected !== undefined && BigInt(size) !== expected)
      throw new Error("Secret read was incomplete");
    return bytes.slice(0, size);
  } finally {
    bytes.fill(0);
  }
}
