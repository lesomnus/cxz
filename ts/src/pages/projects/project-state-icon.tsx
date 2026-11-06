import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import { ContainerState } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { Popover } from "@lesomnus/nox";
import { SessionActivityDots } from "#src/features/session/components/session-activity-dots.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { containerStateName } from "./project-state";
import styles from "./project-state-icon.module.css";

const stateIcons: Record<ContainerState, IconName> = {
  [ContainerState.RUNNING]: "state-running",
  [ContainerState.CREATED]: "state-created",
  [ContainerState.STOPPED]: "state-stopped",
  [ContainerState.ABSENT]: "state-absent",
  [ContainerState.UNSPECIFIED]: "unknown",
};

export function ProjectStateIcon({
  state,
  working,
}: {
  state: ContainerState;
  working?: Session;
}) {
  const label = working ? t("Working") : t(containerStateName(state));
  const descriptions = {
    [ContainerState.RUNNING]: t("The project container is running."),
    [ContainerState.CREATED]: t(
      "The project container exists and has not been started.",
    ),
    [ContainerState.STOPPED]: t(
      "The project container is stopped. Its data is retained.",
    ),
    [ContainerState.ABSENT]: t("This project has no container."),
    [ContainerState.UNSPECIFIED]: t("The container state is unavailable."),
  };
  return (
    <Popover
      content={
        <>
          {working && <div>{t("Working")}</div>}
          {descriptions[state] ?? descriptions[ContainerState.UNSPECIFIED]}
        </>
      }
    >
      <span className={styles.icon} role="img" aria-label={label} tabIndex={0}>
        {working ? (
          <span className="session-indicator">
            <SessionActivityDots session={working} />
          </span>
        ) : (
          <Icon name={stateIcons[state] ?? "unknown"} />
        )}
      </span>
    </Popover>
  );
}
