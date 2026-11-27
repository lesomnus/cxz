import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { InstallationService } from "#gen/cxz/installation_pb";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import type { RoutableConnection } from "#src/shared/api/routable-connections.ts";
import { rpcFetch } from "#src/shared/api/stream-fetch.ts";
import { t } from "#src/shared/i18n/i18n.ts";

function unavailable(error: unknown) {
  const code = ConnectError.from(error).code;
  return code === Code.Unimplemented || code === Code.NotFound;
}

export async function projectAddClient(
  c: Connection,
  target: RoutableConnection,
  signal: AbortSignal,
) {
  const transport =
    target.baseUrl === c.baseUrl
      ? c.transport
      : createConnectTransport({ baseUrl: target.baseUrl, fetch: rpcFetch });
  if (target.baseUrl !== c.baseUrl) {
    const response = await fetch(`${target.baseUrl}/auth/status`, {
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
  return { projects, backend: backend ?? "", baseUrl: target.baseUrl };
}
