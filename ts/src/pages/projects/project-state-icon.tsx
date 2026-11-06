import { ContainerState } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { Popover } from "@lesomnus/nox";
import { SessionActivityDots } from "#src/features/session/components/session-activity-dots.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { containerStateName } from "./project-state";
import styles from "./project-state-icon.module.css";

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
          <svg
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            {state === ContainerState.STOPPED ? (
              <path d="M2.83 9.88a5.5 5.5 0 0 1 7.05-7.05m3.29 3.29a5.5 5.5 0 0 1-7.05 7.05M2.5 13.5l11-11" />
            ) : state === ContainerState.ABSENT ? (
              <circle cx="8" cy="8" r="5.5" strokeDasharray="2 2" />
            ) : (
              <>
                <circle cx="8" cy="8" r="5.5" />
                {state === ContainerState.RUNNING ? (
                  <circle
                    cx="8"
                    cy="8"
                    r="2"
                    fill="currentColor"
                    stroke="none"
                  />
                ) : state === ContainerState.CREATED ? (
                  <path d="M6.25 5.5v5m3.5-5v5" />
                ) : (
                  <path d="M6.5 6.25a1.5 1.5 0 1 1 2 1.4c-.5.2-.5.6-.5 1.1M8 11h.01" />
                )}
              </>
            )}
          </svg>
        )}
      </span>
    </Popover>
  );
}
