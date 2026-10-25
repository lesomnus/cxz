import {
  sessionLocation,
  matchesResource,
} from "#src/shared/navigation/resource-route.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { PanelLink } from "#src/shared/navigation/route-link.tsx";
import { PanelItem } from "@lesomnus/nox";
import { SessionIdentity } from "./session-identity";
import { SessionIndicator } from "./session-indicator";
import { resumableStates } from "#src/features/session/model/session-actions.ts";
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
              {...sessionLocation(s)}
              key={s.runtimeId}
              className={`tree-session ${styles.session} ${matchesResource(s, selected) ? "active" : ""}`}
              data-state={s.status?.state}
              data-unread={
                unread.has(s.runtimeId) || errors.has(s.runtimeId) || undefined
              }
              data-inactive={resumableStates.has(s.status?.state ?? "")}
              pressTarget=".session-heading"
              aria-current={matchesResource(s, selected) ? "true" : undefined}
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
