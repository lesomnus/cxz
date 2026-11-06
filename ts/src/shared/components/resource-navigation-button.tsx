import { forwardRef, type ReactNode } from "react";
import { createLink } from "@tanstack/react-router";
import { ButtonAnchor, type ButtonAnchorProps } from "@lesomnus/nox";
import { Icon } from "#src/shared/icons/icon.tsx";
import styles from "./resource-navigation-button.module.css";
import tableStyles from "./resource-table.module.css";

const ResourceNavigationAnchor = forwardRef<
  HTMLAnchorElement,
  Omit<ButtonAnchorProps, "children"> & {
    name: string;
    alias?: string;
    leading?: ReactNode;
  }
>(function ResourceNavigationAnchor(
  { name, alias, leading, className = "", rowAware = true, ...props },
  ref,
) {
  return (
    <ButtonAnchor
      {...props}
      rowAware={rowAware}
      ref={ref}
      title={name}
      className={`route-link ${tableStyles.layout} ${styles.link} ${className}`}
    >
      {leading}
      <span className={styles.identity}>
        <span className={styles.title}>{name}</span>
        {alias && <span className={styles.subtitle}>{alias}</span>}
      </span>
      <Icon name="open-external" className={styles.arrow} />
    </ButtonAnchor>
  );
});

export const ResourceNavigationButton = createLink(ResourceNavigationAnchor);
