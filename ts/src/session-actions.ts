import type { Client } from "@connectrpc/connect";
import type { SessionService, SessionRef } from "../gen/cxz/session_svc_pb";
import { t } from "./i18n";

export type SessionOperation = "stop" | "resume" | "restart";
type LifecycleClient = Pick<
  Client<typeof SessionService>,
  "get" | "stop" | "resume"
>;
export const resumableStates = new Set(["stopped", "failed", "interrupted"]);

// Restart follows the TUI lifecycle and never adopts a run started by another
// client between Stop and Resume. A failed Stop must not trigger Resume.
export async function manageSession(
  client: LifecycleClient,
  ref: SessionRef,
  operation: SessionOperation,
  expectedRun: string,
) {
  const signal = AbortSignal.timeout(45_000);
  const options = { signal };
  const get = () =>
    client.get({ ref, select: { all: true, project: { all: true } } }, options);
  const assertRun = (run: string | undefined) => {
    if (run !== expectedRun)
      throw new Error(t("Session run changed. Open the menu again."));
  };
  let current = await get();
  assertRun(current.status?.runId);
  const control = () => ({
    ref,
    runId: expectedRun,
    clientId: crypto.randomUUID(),
  });
  if (operation === "stop") return client.stop(control(), options);
  if (
    operation === "restart" &&
    !resumableStates.has(current.status?.state ?? "")
  ) {
    await client.stop(control(), options);
    current = await get();
    assertRun(current.status?.runId);
  }
  return client.resume(control(), options);
}
