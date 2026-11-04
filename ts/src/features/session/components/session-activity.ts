import type { Session } from "#gen/cxz/session_pb";

// Keep the braille-like arrangement, ordered around its perimeter.
export const activityDots = [
  [4, 2],
  [8, 2],
  [8, 6],
  [8, 10],
  [8, 14],
  [4, 14],
  [4, 10],
  [4, 6],
] as const;
const activityColors = ["green", "cyan", "violet"] as const;

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

  let state = seed;
  const random = () => {
    state = (state + 0x6d2b79f5) | 0;
    let value = Math.imul(state ^ (state >>> 15), state | 1);
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61);
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296;
  };
  const phase = random() * activityDots.length;
  const colors = activityDots.map(
    (_, index) => activityColors[index % activityColors.length]!,
  );
  for (let index = colors.length - 1; index > 0; index--) {
    const other = Math.floor(random() * (index + 1));
    [colors[index], colors[other]] = [colors[other]!, colors[index]!];
  }
  return { seed, phase, colors };
}
