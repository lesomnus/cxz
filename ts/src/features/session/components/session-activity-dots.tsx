import { ActivityDots } from "@lesomnus/nox";
import type { Session } from "#gen/cxz/session_pb";
import { sessionActivity } from "./session-activity";
const palette = [
  "var(--aurora-green)",
  "var(--aurora-cyan)",
  "var(--aurora-violet)",
];
export function SessionActivityDots({
  session,
  className,
}: {
  session: Pick<Session, "id" | "runtimeId" | "status">;
  className?: string;
}) {
  const { seed } = sessionActivity(session);
  return (
    <ActivityDots
      key={seed}
      seed={seed}
      palette={palette}
      className={className}
      dotClassName="session-activity-dot"
    />
  );
}
