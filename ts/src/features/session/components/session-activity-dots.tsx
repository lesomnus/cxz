import type { CSSProperties } from "react";
import type { Session } from "#gen/cxz/session_pb";
import { activityDots, sessionActivity } from "./session-activity";

export function SessionActivityDots({
  session,
  className,
}: {
  session: Pick<Session, "id" | "runtimeId" | "status">;
  className?: string;
}) {
  const activity = sessionActivity(session);
  return (
    <svg
      key={activity.seed}
      className={className}
      width="12"
      height="16"
      viewBox="0 0 12 16"
      aria-hidden="true"
    >
      {activityDots.map(([cx, cy], index) => (
        <circle
          className="session-activity-dot"
          key={index}
          cx={cx}
          cy={cy}
          r="1.3"
          style={
            {
              animationDelay: `calc((${index} + ${activity.phase}) * var(--session-dot-step) * -1)`,
              "--session-dot-highlight": `var(--aurora-${activity.colors[index]})`,
            } as CSSProperties
          }
        />
      ))}
    </svg>
  );
}
