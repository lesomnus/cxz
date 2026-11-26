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

// `outdated` is a runtime older than the manager: it keeps working, and is
// replaced when the project next starts -- on Kubernetes also by cxz once the
// project's sessions are idle.
export function ProjectStateIcon({
  state,
  outdated = false,
}: {
  state: ContainerState;
  outdated?: boolean;
}) {
  const label = outdated
    ? t("{state}, runtime update pending", {
        state: t(containerStateName(state)),
      })
    : t(containerStateName(state));
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
          {descriptions[state] ?? descriptions[ContainerState.UNSPECIFIED]}
          {outdated && (
            <>
              {" "}
              {t(
                "Its cxz runtime is older than the manager's and is replaced when the project next starts.",
              )}
            </>
          )}
        </>
      }
    >
      <span
        className={styles.icon}
        role="img"
        aria-label={label}
        tabIndex={0}
        data-outdated={outdated || undefined}
      >
        <Icon name={stateIcons[state] ?? "unknown"} />
      </span>
    </Popover>
  );
}
