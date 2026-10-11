import { ref, type Connection } from "#src/shared/api/connection.ts";
import { t } from "#src/shared/i18n/i18n.ts";

export async function containerDownload(
  client: Pick<Connection["projects"], "download">,
  project: string,
  path: string,
  signal: AbortSignal,
) {
  const parts: ArrayBuffer[] = [];
  let received = 0n,
    expected: bigint | undefined;
  for await (const reply of client.download(
    { ref: ref(project), path },
    { signal, timeoutMs: 120000 },
  )) {
    if (reply.totalSize > 0n || (expected === undefined && !reply.data.length))
      expected = reply.totalSize;
    received += BigInt(reply.data.length);
    if (reply.totalSize < 0n || (expected !== undefined && received > expected))
      throw new Error(t("File download was incomplete"));
    if (reply.data.length) parts.push(new Uint8Array(reply.data).buffer);
  }
  signal.throwIfAborted();
  if (expected !== undefined && received !== expected)
    throw new Error(t("File download was incomplete"));
  return new Blob(parts, { type: "application/octet-stream" });
}

export function saveContainerDownload(blob: Blob, path: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = path.split("/").at(-1) || "download";
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
