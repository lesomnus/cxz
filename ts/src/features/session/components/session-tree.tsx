import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { PanelLink } from "#src/shared/navigation/route-link.tsx";
import { PanelItem } from "@lesomnus/nox";
import { SessionIdentity } from "./session-identity";
import { resumableStates } from "#src/features/session/model/session-actions.ts";
import { UnreadBadge } from "#src/shared/notifications/unread-badge.tsx";
import { useState, type CSSProperties } from "react";
import styles from "./session-tree.module.css";

export function SessionTreeGroup({
  project,
  items,
  loading,
  selected,
  open,
  toggle,
  unread = new Set<string>(),
  errors = new Set<string>(),
  background = new Set<string>(),
}: {
  project: Project;
  items: Session[];
  loading: boolean;
  selected: string;
  open: boolean;
  toggle: () => void;
  unread?: ReadonlySet<string>;
  errors?: ReadonlySet<string>;
  background?: ReadonlySet<string>;
}) {
  useLocale();
  return (
    <section
      className={`session-tree-group ${styles.group}`}
      aria-label={project.name || project.alias}
    >
      <PanelItem
        className={`tree-project ${styles.project}`}
        aria-expanded={open}
        onClick={toggle}
        data-unread={
          (!open &&
            items.some(
              (s) => unread.has(s.runtimeId) || errors.has(s.runtimeId),
            )) ||
          undefined
        }
      >
        <span>{project.name || project.alias}</span>
      </PanelItem>
      {open && (
        <div className="tree-sessions">
          {items.map((s) => (
            <PanelLink
              to="/sessions/$sessionId"
              params={{ sessionId: s.runtimeId }}
              key={s.runtimeId}
              className={`tree-session ${styles.session} ${selected === s.runtimeId ? "active" : ""}`}
              data-state={s.status?.state}
              data-unread={
                unread.has(s.runtimeId) || errors.has(s.runtimeId) || undefined
              }
              data-inactive={resumableStates.has(s.status?.state ?? "")}
              pressTarget=".session-heading"
              aria-current={selected === s.runtimeId ? "true" : undefined}
            >
              <SessionIndicator
                session={s}
                unread={unread.has(s.runtimeId)}
                error={errors.has(s.runtimeId)}
                background={background.has(s.runtimeId)}
              />
              <SessionIdentity session={s} />
            </PanelLink>
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
const activityColors = ["green", "cyan", "violet"] as const;

function randomActivityColors() {
  // Keep all toolbar hues present, with a stable random order per indicator.
  const colors = activityDots.map(
    (_, index) => activityColors[index % activityColors.length]!,
  );
  for (let index = colors.length - 1; index > 0; index--) {
    const other = Math.floor(Math.random() * (index + 1));
    [colors[index], colors[other]] = [colors[other]!, colors[index]!];
  }
  return colors;
}

function SessionIndicator({
  session,
  unread,
  error,
  background,
}: {
  session: Session;
  unread: boolean;
  error: boolean;
  background: boolean;
}) {
  useLocale();
  const [dotColors] = useState(randomActivityColors);
  const state = session.status?.state || "unknown";
  if (error)
    return (
      <span className="session-indicator" title={t("Unread error")}>
        <UnreadBadge error />
      </span>
    );
  if (resumableStates.has(state))
    return <span className="session-indicator" aria-hidden="true" />;
  const question = !!session.status?.pending.length;
  const working = ["running", "working", "waiting_input"].includes(state);
  if (unread)
    return (
      <span className="session-indicator" title={t("Unread response")}>
        <UnreadBadge />
      </span>
    );
  return (
    <span
      className={`session-indicator ${question ? "attention" : ""}`}
      role="img"
      aria-label={
        question
          ? t("Awaiting answer")
          : !working && background
            ? t("Background tasks")
            : state
      }
    >
      {question ? (
        "?"
      ) : working || background ? (
        <svg
          className={
            !working && background ? styles.backgroundActivity : undefined
          }
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
                  animationDelay: `calc(${index} * var(--session-dot-step) * -1)`,
                  "--session-dot-highlight": `var(--aurora-${dotColors[index]})`,
                } as CSSProperties
              }
            />
          ))}
        </svg>
      ) : (
        " "
      )}
    </span>
  );
}
