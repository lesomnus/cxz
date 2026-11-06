import {
  PageToolbar,
  Section,
  SectionGroup,
  SectionLayout,
} from "@lesomnus/nox";
import type { ComponentProps, ComponentPropsWithRef } from "react";
import pageStyles from "./resource-page.module.css";
import dashboardStyles from "./resource-dashboard.module.css";

/** Own the width derivation and named container without transitive CSS composition. */
export function ResourcePage({
  className = "",
  ...props
}: ComponentPropsWithRef<"main">) {
  return (
    <main
      {...props}
      className={`${pageStyles.page} ${className}`}
      data-resource-page
    />
  );
}

export function ResourceToolbar({
  children,
  actions,
  className = "",
  ...props
}: ComponentProps<typeof PageToolbar>) {
  return (
    <PageToolbar {...props} className={`${pageStyles.toolbar} ${className}`}>
      <div className={pageStyles.toolbarContent} data-resource-toolbar-content>
        {children}
        {actions && <div className={pageStyles.toolbarActions}>{actions}</div>}
      </div>
    </PageToolbar>
  );
}

/** Resource entries share the toolbar's width and responsive column policy. */
export function ResourceLayout({
  className = "",
  ...props
}: ComponentPropsWithRef<"div">) {
  return (
    <SectionLayout
      {...props}
      className={`${pageStyles.content} ${className}`}
      data-resource-layout
    />
  );
}

export function ResourceDashboard({
  actionsFirstOnNarrow = false,
  className = "",
  ...props
}: ComponentPropsWithRef<"div"> & { actionsFirstOnNarrow?: boolean }) {
  return (
    <div
      {...props}
      className={`${dashboardStyles.layout} ${className}`}
      data-resource-dashboard
      data-actions-first={actionsFirstOnNarrow || undefined}
    />
  );
}

export function ResourceDashboardColumn({
  kind,
  className = "",
  ...props
}: ComponentPropsWithRef<"div"> & { kind: "information" | "activity" }) {
  return (
    <SectionGroup
      {...props}
      className={`${dashboardStyles[kind]} ${className}`}
      data-resource-dashboard-column={kind}
    />
  );
}

export function ResourceSessionSection({
  className = "",
  ...props
}: ComponentPropsWithRef<"section">) {
  return (
    <Section
      {...props}
      className={`${dashboardStyles.sessions} ${className}`}
    />
  );
}
