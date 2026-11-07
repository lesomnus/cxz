import { forwardRef } from "react";
import type { ButtonAnchorProps } from "@lesomnus/nox";
import type { Session } from "#gen/cxz/session_pb";
import { sessionLocation } from "#src/shared/navigation/resource-route.ts";
import { AgentBrand } from "./agent-brand";
import { ResourceNavigationButton } from "./resource-navigation-button";
import styles from "./session-navigation-button.module.css";

/** Session resource cards share their identity, logo spacing and navigation. */
export const SessionNavigationButton = forwardRef<
  HTMLAnchorElement,
  Omit<ButtonAnchorProps, "children" | "href"> & {
    session: Session;
    annotation?: string;
    search?: { seq?: string };
  }
>(function SessionNavigationButton(
  { session, annotation, className = "", ...props },
  ref,
) {
  const name = session.name || session.alias || session.runtimeId;
  const alias = session.alias || session.runtimeId;
  return (
    <ResourceNavigationButton
      {...props}
      {...sessionLocation(session)}
      ref={ref}
      data-session-navigation=""
      className={`${styles.button} ${className}`}
      name={name}
      alias={alias}
      annotation={annotation}
      title={annotation ? `${name}\n${alias}\n${annotation}` : name}
      leading={
        <span className={styles.logo} aria-hidden="true">
          <AgentBrand agent={session.agent} />
        </span>
      }
    />
  );
});
