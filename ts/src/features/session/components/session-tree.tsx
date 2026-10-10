import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { Button } from "@lesomnus/cxz-ui";
import { SessionIdentity } from "./session-identity";

export function SessionTreeGroup({
  project,
  items,
  loading,
  selected,
  open,
  toggle,
  unread = new Set<string>(),
}: {
  project: Project;
  items: Session[];
  loading: boolean;
  selected: string;
  open: boolean;
  toggle: () => void;
  unread?: ReadonlySet<string>;
}) {
  useLocale();
  return (
    <section
      className="session-tree-group"
      aria-label={project.name || project.alias}
    >
      <Button className="tree-project" aria-expanded={open} onClick={toggle}>
        <span>{project.name || project.alias}</span>
      </Button>
      {open && (
        <div className="tree-sessions">
          {items.map((s) => (
            <RouteLink
              to="/sessions/$sessionId"
              params={{ sessionId: s.runtimeId }}
              key={s.runtimeId}
              className={`tree-session ${selected === s.runtimeId ? "active" : ""}`}
              pressTarget=".session-heading"
              aria-current={selected === s.runtimeId ? "true" : undefined}
            >
              <SessionIndicator session={s} />
              <SessionIdentity session={s} />
              {unread.has(s.runtimeId) && (
                <span
                  className="session-unread"
                  role="img"
                  aria-label={t("Unread response")}
                  title={t("Unread response")}
                >
                  +
                </span>
              )}
            </RouteLink>
          ))}
          {loading && !items.length && (
            <small className="tree-message">{t("Loading…")}</small>
          )}
          {!loading && !items.length && (
            <small className="tree-message">{t("No sessions")}</small>
          )}
        </div>
      )}
    </section>
  );
}

// Keep the braille-like arrangement, ordered around its perimeter.
const activityDots = [
  [4, 2],
  [8, 2],
  [8, 6],
  [8, 10],
  [8, 14],
  [4, 14],
  [4, 10],
  [4, 6],
] as const;

function SessionIndicator({ session }: { session: Session }) {
  useLocale();
  const state = session.status?.state || "unknown";
  const question = !!session.status?.pending.length;
  const working = ["running", "working", "waiting_input"].includes(state);
  return (
    <span
      className={`session-indicator ${question ? "attention" : ""}`}
      role="img"
      aria-label={question ? t("Awaiting answer") : state}
    >
      {question ? (
        "?"
      ) : working ? (
        <svg width="12" height="16" viewBox="0 0 12 16" aria-hidden="true">
          {activityDots.map(([cx, cy], index) => (
            <circle
              className="session-activity-dot"
              key={index}
              cx={cx}
              cy={cy}
              r="1.3"
              style={{
                animationDelay: `calc(${index} * var(--session-dot-step) * -1)`,
              }}
            />
          ))}
        </svg>
      ) : state === "stopped" ? (
        "○"
      ) : (
        " "
      )}
    </span>
  );
}
