import { forwardRef } from "react";
import { createLink } from "@tanstack/react-router";
import { LinkButton, Skeleton, type TableLinkButtonProps } from "@lesomnus/nox";
import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import styles from "./resource-navigation-button.module.css";
import tableStyles from "./resource-table.module.css";

const ResourceNavigationAnchor = forwardRef<
  HTMLAnchorElement,
  Omit<TableLinkButtonProps, "variant" | "children" | "detail" | "trailing"> & {
    name: string;
    alias?: string;
    annotation?: string;
    navigationIcon?: IconName;
  }
>(function ResourceNavigationAnchor(
  {
    name,
    alias,
    annotation,
    navigationIcon = "open-external",
    className = "",
    ...props
  },
  ref,
) {
  return (
    <LinkButton
      {...props}
      variant="table"
      ref={ref}
      title={props.title ?? name}
      className={`route-link ${tableStyles.layout} ${styles.link} ${className}`}
      detail={
        alias || annotation ? (
          <>
            {alias}
            {alias && annotation && " · "}
            {annotation && <span>{annotation}</span>}
          </>
        ) : undefined
      }
      trailing={<Icon name={navigationIcon} />}
    >
      {name}
    </LinkButton>
  );
});

export const ResourceNavigationButton = createLink(ResourceNavigationAnchor);

export function ResourceNavigationPlaceholder() {
  return (
    <Skeleton
      variant="rect"
      height="var(--resource-table-control-height)"
      className={`${tableStyles.layout} ${styles.link}`}
    />
  );
}
