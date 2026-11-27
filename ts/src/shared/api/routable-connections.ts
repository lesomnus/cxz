import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { Id } from "@lesomnus/payday/pdid";
import type { Connection } from "#gen/cxz/connection_pb";
import { ConnectionService } from "#gen/cxz/connection_svc_pb";

export const standaloneConnection = "@local";

export type RoutableConnection = {
  alias: string;
  handle: string;
  id?: string;
  baseUrl: string;
  primary: boolean;
  entry?: Connection;
};

export function localConnection(origin: string): RoutableConnection {
  return {
    alias: "local",
    handle: standaloneConnection,
    baseUrl: origin,
    primary: true,
  };
}

export function defaultConnection(connections: readonly RoutableConnection[]) {
  return connections.find((connection) => connection.primary) ?? connections[0];
}

export function resolveConnection(
  connections: readonly RoutableConnection[],
  handle: string,
) {
  return connections.find((connection) =>
    handle.startsWith("@")
      ? connection.handle === handle
      : connection.id === handle.toLowerCase(),
  );
}

export function webConnectionCatalog(origin: string) {
  const catalog = createClient(
    ConnectionService,
    createConnectTransport({ baseUrl: origin }),
  );
  let pending: Promise<RoutableConnection[]> | undefined;
  return () =>
    (pending ??= read().catch((error: unknown) => {
      pending = undefined;
      throw error;
    }));

  async function read() {
    try {
      const { items } = await catalog.list({}, { timeoutMs: 8000 });
      return items.flatMap((item): RoutableConnection[] => {
        if (!/^[a-z0-9][a-z0-9-]{0,39}$/.test(item.name)) return [];
        if (
          item.transport !== "gateway" &&
          !(item.transport === "origin" && item.target === origin)
        )
          return [];
        const handle = `@${item.name}`;
        const id =
          item.id.length === 16 ? new Id(item.id).toString() : undefined;
        return [
          {
            alias: item.name,
            handle,
            id,
            baseUrl:
              item.transport === "gateway"
                ? `${origin}/c/${id ?? handle}`
                : origin,
            primary: item.isDefault,
            entry: item,
          },
        ];
      });
    } catch (error) {
      if (
        ![Code.Unimplemented, Code.NotFound, Code.Unauthenticated].includes(
          ConnectError.from(error).code,
        )
      )
        throw error;
      return [localConnection(origin)];
    }
  }
}
