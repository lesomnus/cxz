import type { ComponentProps, ReactNode } from "react";
import { ValueMenu } from "@lesomnus/nox";
import {
  ResourceLayout,
  ResourceToolbar,
} from "#src/shared/components/resource-layout.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import type { SettingsNavigation } from "./use-settings-navigation";
import styles from "./settings-content.module.css";

const topics = () => [
  { value: "general", label: t("General") },
  { value: "editor", label: t("Editor") },
  { value: "connections", label: t("Connections") },
];
export function SettingsToolbar({
  navigation,
  children,
  className = "",
  ...props
}: ComponentProps<typeof ResourceToolbar> & {
  navigation: SettingsNavigation;
}) {
  return (
    <ResourceToolbar {...props} className={`${styles.toolbar} ${className}`}>
      {navigation.compact && (
        <div className={styles.topicMenu}>
          <ValueMenu
            label={t("Settings topics")}
            value={navigation.active}
            options={topics()}
            choose={navigation.choose}
          />
        </div>
      )}
      {children}
    </ResourceToolbar>
  );
}
export function SettingsContent({
  children,
  navigation,
}: {
  children: ReactNode;
  navigation: SettingsNavigation;
}) {
  return (
    <ResourceLayout
      ref={navigation.layout}
      className={styles.layout}
      data-compact={navigation.compact || undefined}
    >
      <nav
        className={styles.navigation}
        aria-label={t("Settings topics")}
        data-settings-navigation
        aria-hidden={navigation.compact || undefined}
        inert={navigation.compact}
      >
        {topics().map(({ value, label }) => (
          <button
            type="button"
            key={value}
            className={styles.link}
            aria-current={navigation.active === value ? "location" : undefined}
            aria-controls={`settings-${value}`}
            onClick={() => navigation.choose(value)}
          >
            {label}
          </button>
        ))}
      </nav>
      <div className={styles.content}>{children}</div>
    </ResourceLayout>
  );
}
