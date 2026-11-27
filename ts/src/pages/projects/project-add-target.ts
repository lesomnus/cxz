import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { ConnectionService } from "#gen/cxz/connection_svc_pb";
import { InstallationService } from "#gen/cxz/installation_pb";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import {
  namedGatewayStatus,
  namedGatewayUrl,
} from "#src/shared/api/named-gateway.ts";
import { rpcFetch } from "#src/shared/api/stream-fetch.ts";

export type ProjectAddTarget = {
  name: string;
  connection: string;
  baseUrl: string;
};
export type ProjectBackend = "docker" | "kubernetes";

function unavailable(error: unknown) {
  const code = ConnectError.from(error).code;
  return code === Code.Unimplemented || code === Code.NotFound;
}

export async function projectAddTargets(c: Connection, signal: AbortSignal) {
  const catalog = createClient(
    ConnectionService,
    c.inProcess
      ? c.transport
      : createConnectTransport({ baseUrl: location.origin }),
  );
  try {
    const { items } = await catalog.list({}, { signal, timeoutMs: 10000 });
    const targets = items.flatMap((item): ProjectAddTarget[] => {
      const baseUrl =
        item.transport === "gateway" && namedGatewayUrl(item.name);
      if (!baseUrl) return [];
      const useDefault = item.isDefault && baseUrl !== c.baseUrl;
      return [
        {
          name: item.name,
          connection: useDefault ? "_" : item.name,
          baseUrl: useDefault ? location.origin : baseUrl,
        },
      ];
    });
    if (targets.length) return targets;
  } catch (error) {
    if (!unavailable(error)) throw error;
  }
  return [
    {
      name: new URL(c.baseUrl, location.href).host,
      connection: "_",
      baseUrl: c.baseUrl,
    },
  ];
}

export async function projectAddClient(
  c: Connection,
  target: ProjectAddTarget,
  signal: AbortSignal,
) {
  const transport =
    target.baseUrl === c.baseUrl
      ? c.transport
      : createConnectTransport({ baseUrl: target.baseUrl, fetch: rpcFetch });
  if (target.baseUrl !== c.baseUrl)
    await namedGatewayStatus(
      target.baseUrl,
      AbortSignal.any([signal, AbortSignal.timeout(10000)]),
    );
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
