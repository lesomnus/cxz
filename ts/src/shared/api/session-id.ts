import { Id } from "@lesomnus/payday/pdid";

// Keep effect/cache identity stable when a new catalog message carries the
// same UUID bytes in a different Uint8Array.
export const sessionUUID = (id?: Uint8Array) =>
  id?.length === 16 ? new Id(id).toString() : "";

export const sessionRef = (id: Uint8Array) => ({
  key: { case: "id" as const, value: id },
});
