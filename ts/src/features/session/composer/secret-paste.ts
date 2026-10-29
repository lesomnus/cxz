import { ref, type Connection } from "#src/shared/api/connection.ts";
import type { ComposerPaste } from "./composer-pastes";

export type SecretFiles = {
  session: string;
  run: string;
  put: (bytes: Uint8Array, signal: AbortSignal) => Promise<string>;
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
    put: async (bytes, signal) => {
      const result = await c.projects.putSecretFile(
        { ref: ref(project), session: `${session}/${run}`, secret: bytes },
        { signal, timeoutMs: 30000 },
      );
      if (!result.path?.startsWith("/"))
        throw new Error("Secret path unavailable");
      return result.path;
    },
    remove: async (path) => {
      await c.projects.deleteSecretFile(
        { ref: ref(project), path },
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
  async function save(signal: AbortSignal) {
    let path = "";
    try {
      signal.throwIfAborted();
      if (bytes.length > 65536) throw new Error("Secret limit: 64 KiB");
      if (!files) throw new Error("Secret paste requires an active session.");
      path = await files.put(bytes, signal);
      signal.throwIfAborted();
      secret.path = path;
      secret.state = "ready";
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
