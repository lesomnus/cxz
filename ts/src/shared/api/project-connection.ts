import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { InstallationService } from "#gen/cxz/installation_pb";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { rpcFetch } from "#src/shared/api/stream-fetch.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { t } from "#src/shared/i18n/i18n.ts";

function unavailable(error: unknown) {
  const code = ConnectError.from(error).code;
  return code === Code.Unimplemented || code === Code.NotFound;
}

async function projectConnection(
  c: Connection,
  baseUrl: string,
  signal: AbortSignal,
) {
  const transport =
    baseUrl === c.baseUrl
      ? c.transport
      : createConnectTransport({ baseUrl, fetch: rpcFetch });
  if (baseUrl !== c.baseUrl) {
    const response = await fetch(`${baseUrl}/auth/status`, {
      signal: AbortSignal.any([signal, AbortSignal.timeout(10000)]),
    });
    if (
      !response.ok ||
      response.headers.get("content-type")?.includes("text/html")
    )
      throw new Error(t("Cannot reach cxz"));
  }
  const installation = createClient(InstallationService, transport);
  const projects = createClient(ProjectService, transport);
  let backend: string | undefined;
  try {
    ({ projectBackend: backend } = await installation.version(
      {},
      { signal, timeoutMs: 10000 },
    ));
  } catch (error) {
    if (!unavailable(error)) throw error;
  }
  return { projects, backend: backend ?? "", baseUrl };
}

export function useProjectConnection(
  c: Connection,
  baseUrl?: string,
  { cached = false } = {},
) {
  return useRemote(
    (signal) => projectConnection(c, baseUrl!, signal),
    [c, baseUrl],
    {
      enabled: !!baseUrl,
      cache: cached
        ? {
            store: c.reads,
            key: `project-connection/${baseUrl}`,
            ttl: 5 * 60_000,
          }
        : undefined,
    },
  );
}
