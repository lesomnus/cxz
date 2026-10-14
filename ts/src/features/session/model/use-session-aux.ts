import { useMemo, useSyncExternalStore } from "react";
import type { Connection } from "#src/shared/api/connection.ts";

// Both the transcript and auxiliary controls share one connection-owned
// AuxEvents stream. Navigation and transient disconnects retain its snapshot.
export function useSessionAux(c: Connection, id: string) {
  const aux = useMemo(() => c.conversations.get(id).aux, [c, id]);
  return useSyncExternalStore(aux.subscribe, aux.snapshot);
}
