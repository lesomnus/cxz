import { createRouterTransport, type Transport } from "@connectrpc/connect";
import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { ConnectionService } from "#gen/cxz/connection_svc_pb";
import { ConnectionSchema, type Connection } from "#gen/cxz/connection_pb";

// Where this browser is attached, and where else it has been told it could
// attach. The manager answers Unimplemented for this service, because a
// connection is a fact about the client: a server does not know what name it
// was dialled by. So the browser serves it to itself, out of this origin and
// local storage. See docs/plans/web-connections.md.
//
// Only what a person typed is stored -- a name and an origin. No credential,
// and no token path: a connection reaches an installation whole, so what it
// says is where to attach, never how to prove it. Credentials stay where the
// browser already keeps them, in that origin's own HttpOnly cookie, which this
// code cannot read and does not want to.
export const storageKey = "cxz.connections";

type Saved = { name: string; target: string };

function saved(storage: Storage): Saved[] {
  const raw = storage.getItem(storageKey);
  if (!raw) return [];
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    // Anything beyond a name and a target is dropped rather than carried: a
    // credential that got in here by some other version is not kept alive by
    // this one.
    return parsed
      .filter(
        (v): v is Saved =>
          !!v &&
          typeof (v as Saved).name === "string" &&
          typeof (v as Saved).target === "string",
      )
      .map(({ name, target }) => ({ name, target }));
  } catch {
    return [];
  }
}

function connection(v: Saved & Partial<Connection>): Connection {
  return create(ConnectionSchema, {
    name: v.name,
    target: v.target,
    transport: v.transport ?? "",
    isDefault: v.isDefault ?? false,
    readOnly: v.readOnly ?? false,
  });
}

// here is the connection a page cannot be without: the origin it was served
// from. It is read-only because forgetting it would mean forgetting how the
// page got here, and it is the default because it is the one already attached.
export function here(origin: string, name?: string): Connection {
  return connection({
    name: name || new URL(origin).host,
    target: origin,
    transport: "origin",
    isDefault: true,
    readOnly: true,
  });
}

export function list(
  origin: string = location.origin,
  storage: Storage = localStorage,
): Connection[] {
  const own = here(origin);
  const rest = saved(storage)
    .filter((v) => v.target !== origin)
    .map((v) => connection({ ...v, transport: transportOf(v.target) }));
  return [own, ...rest];
}

export function remember(
  entry: Saved,
  origin: string = location.origin,
  storage: Storage = localStorage,
) {
  if (entry.target === origin) return;
  const rest = saved(storage).filter((v) => v.target !== entry.target);
  storage.setItem(
    storageKey,
    JSON.stringify([...rest, { name: entry.name, target: entry.target }]),
  );
}

export function forget(
  target: string,
  origin: string = location.origin,
  storage: Storage = localStorage,
) {
  if (target === origin) return;
  storage.setItem(
    storageKey,
    JSON.stringify(saved(storage).filter((v) => v.target !== target)),
  );
}

// What a target will be attached by, read from how it is written. The browser
// can only attach to an origin; the other schemes are what the TUI on that
// machine would use, and are shown as themselves rather than translated.
export function transportOf(target: string): string {
  const scheme = target.slice(0, Math.max(0, target.indexOf("://")));
  if (scheme === "http" || scheme === "https") return "origin";
  return scheme;
}

// transport serves the contract in this tab. The UI reads connections through a
// generated client, exactly as it reads a service the manager implements, so
// that moving this answer to a server later changes the transport and nothing
// else.
export function transport(
  origin: string = location.origin,
  storage: Storage = localStorage,
): Transport {
  return createRouterTransport((router) => {
    router.service(ConnectionService, {
      list: () => ({ items: list(origin, storage) }),
    });
  });
}

export function client(
  origin: string = location.origin,
  storage: Storage = localStorage,
) {
  return createClient(ConnectionService, transport(origin, storage));
}
