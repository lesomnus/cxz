import { Section, SectionGroup, SectionLayout } from "@lesomnus/nox";
import type { ComponentPropsWithRef } from "react";
import pageStyles from "./resource-page.module.css";
import dashboardStyles from "./resource-dashboard.module.css";

/** Resource entries share the toolbar's width and responsive column policy. */
export function ResourceLayout({
  className = "",
  ...props
}: ComponentPropsWithRef<"div">) {
  return (
    <SectionLayout
      {...props}
      className={`${pageStyles.content} ${className}`}
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
