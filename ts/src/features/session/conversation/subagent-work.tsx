import type { ReactNode } from "react";
import { Button, DurationDisplay } from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  subagentDurationMs,
  type SubagentGroup,
} from "#src/features/session/model/subagent-groups.ts";
import styles from "./subagent-work.module.css";

// One subagent's work, folded. Its replies arrive on the session's own stream,
// so left open they read as the session talking to itself -- and with three
// running at once, as three sessions talking over each other. Folded, the
// conversation keeps one row per subagent in the order they were started, and
// opening one puts its own replies back in their own order.
export function SubagentWork({
  group,
  expanded,
  toggle,
  render,
}: {
  group: SubagentGroup;
  expanded: boolean;
  toggle: () => void;
  render: (event: SubagentGroup["events"][number]) => ReactNode;
}) {
  useLocale();
  const duration = subagentDurationMs(group);
  const name = group.type || t("subagent");
  return (
    <section
      className={styles.group}
      data-expanded={expanded || undefined}
      aria-label={t("Subagent {name}", { name })}
    >
      <Button
        type="button"
        className={styles.header}
        aria-expanded={expanded}
        onClick={toggle}
      >
        {/* The caret the file tree and the session groups use, for the same
            kind of disclosure. */}
        <span className={styles.caret} aria-hidden="true">
          {expanded ? "⌄" : "›"}
        </span>
        <span className={styles.name}>{name}</span>
        <span className={styles.counts}>
          {group.messages > 0 &&
            t(group.messages === 1 ? "{count} message" : "{count} messages", {
              count: group.messages,
            })}
          {group.messages > 0 && group.work > 0 && " · "}
          {group.work > 0 &&
            t(group.work === 1 ? "{count} step" : "{count} steps", {
              count: group.work,
            })}
        </span>
        {duration !== undefined && (
          <span className={styles.duration}>
            <DurationDisplay durationMs={duration} />
          </span>
        )}
      </Button>
      {expanded && (
        <div className={styles.body}>
          {group.events.map((event) => (
            <div key={event.seq.toString()} className={styles.row}>
              {render(event)}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
