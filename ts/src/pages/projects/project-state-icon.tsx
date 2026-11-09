import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import { ContainerState } from "#gen/cxz/project_pb";
import { Popover } from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import { containerStateName } from "./project-state";
import styles from "./project-state-icon.module.css";

const stateIcons: Record<ContainerState, IconName> = {
  [ContainerState.RUNNING]: "state-running",
  [ContainerState.CREATED]: "state-created",
  [ContainerState.STOPPED]: "stop",
  [ContainerState.ABSENT]: "state-absent",
  [ContainerState.UNSPECIFIED]: "unknown",
};

export function ProjectStateIcon({ state }: { state: ContainerState }) {
  const label = t(containerStateName(state));
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
      content={descriptions[state] ?? descriptions[ContainerState.UNSPECIFIED]}
    >
      <span className={styles.icon} role="img" aria-label={label} tabIndex={0}>
        <Icon name={stateIcons[state] ?? "unknown"} />
      </span>
    </Popover>
  );
}
