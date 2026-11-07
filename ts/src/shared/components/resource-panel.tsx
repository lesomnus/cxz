import type { ComponentProps, ReactNode } from "react";
import {
  NavigationPanelRoot,
  NavigationPanelHeader,
  NavigationPanelTitle,
  NavigationPanelActions,
  NavigationPanelScrollBody,
} from "@lesomnus/nox";
import { PanelScroll } from "./panel-scroll";
import styles from "./resource-panel.module.css";
export function ResourcePanel({
  title,
  actions,
  children,
  className = "",
  unreadSelector,
  ...props
}: Omit<ComponentProps<"aside">, "title"> & {
  title: ReactNode;
  actions?: ReactNode;
  unreadSelector?: string;
}) {
  return (
    <NavigationPanelRoot
      {...props}
      className={`resource-panel ${styles.panel} ${className}`}
    >
      <NavigationPanelHeader>
        <NavigationPanelTitle>{title}</NavigationPanelTitle>
        {actions && <NavigationPanelActions>{actions}</NavigationPanelActions>}
      </NavigationPanelHeader>
      <NavigationPanelScrollBody
        as={PanelScroll}
        data-unread-selector={unreadSelector}
      >
        {children}
      </NavigationPanelScrollBody>
    </NavigationPanelRoot>
  );
}
