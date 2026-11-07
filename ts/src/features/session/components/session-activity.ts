import type { Session } from "#gen/cxz/session_pb";

import { activityPattern } from "@lesomnus/nox";
export { activityDots } from "@lesomnus/nox";
export const isSessionWorking = (session: Pick<Session, "status">) =>
  ["running", "working", "waiting_input"].includes(session.status?.state ?? "");

export function sessionActivity(
  session: Pick<Session, "id" | "runtimeId" | "status">,
) {
  let seed = 2166136261;
  const hash = (value: number) => {
    seed = Math.imul(seed ^ value, 16777619);
  };
  if (session.id.length) {
    hash(0);
    for (const byte of session.id) hash(byte);
  } else {
    hash(1);
    for (const character of session.runtimeId) hash(character.charCodeAt(0));
  }
  hash(2);
  for (const character of session.status?.runId ?? "")
    hash(character.charCodeAt(0));
  seed >>>= 0;

  return activityPattern(seed, ["green", "cyan", "violet"]);
}
