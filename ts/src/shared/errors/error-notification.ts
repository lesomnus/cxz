import { createContextKey, createContextValues } from "@connectrpc/connect";

/** An explicit per-call override; unset leaves notification policy to the caller. */
export const notifyError = createContextKey<boolean | undefined>(undefined, {
  description: "Include captured RPC failures in sidebar alerts",
});

export function errorNotificationOptions(notify: boolean) {
  return { contextValues: createContextValues().set(notifyError, notify) };
}
