import { useNavigate } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { Icon } from "#src/shared/icons/icon.tsx";
import { projectIconName } from "#src/shared/components/project-icon.ts";
import {
  matchesResource,
  resourceHandle,
  useResourceRoutes,
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
  collapsible = true,
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
  collapsible?: boolean;
  unread?: ReadonlySet<string>;
  errors?: ReadonlySet<string>;
  background?: ReadonlySet<string>;
}) {
  const { projectLocation, sessionLocation } = useResourceRoutes();
  useLocale();
  const navigate = useNavigate();
  const expanded = !collapsible || open;
  const pendingToggle = useRef<number>(undefined);
  useEffect(() => () => window.clearTimeout(pendingToggle.current), []);
  const openProject = () =>
    void navigate(projectLocation(resourceHandle(project)));
  return (
    <section
      className={`session-tree-group ${styles.group}`}
      aria-label={project.name || project.alias}
    >
      <PanelItem
        className={`tree-project ${styles.project}`}
        aria-expanded={collapsible ? open : undefined}
        onClick={(event) => {
          if (event.detail === 0) {
            if (collapsible) toggle();
            else openProject();
          } else if (collapsible && event.detail === 1) {
            // A first click can move the heading when the shortened list scrolls.
            window.clearTimeout(pendingToggle.current);
            pendingToggle.current = window.setTimeout(toggle, 300);
          }
        }}
        onDoubleClick={() => {
          window.clearTimeout(pendingToggle.current);
          openProject();
        }}
        data-unread={
          (!expanded &&
            items.some(
              (s) => unread.has(s.runtimeId) || errors.has(s.runtimeId),
            )) ||
          undefined
        }
      >
        <Icon
          className={styles.projectIcon}
          name={projectIconName(project) ?? "projects"}
          size={11}
        />
        <span>{project.name || project.alias}</span>
      </PanelItem>
      {expanded && (
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
