import type { ComponentProps, ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import {
  ResourceLayout,
  ResourceToolbar,
} from "#src/shared/components/resource-layout.tsx";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import styles from "./settings-content.module.css";

export function SettingsToolbar(props: ComponentProps<typeof ResourceToolbar>) {
  return <ResourceToolbar {...props} className={styles.toolbar} />;
}

export function SettingsContent({ children }: { children: ReactNode }) {
  const { settingsTopic } = useWorkspaceRoute();
  return (
    <ResourceLayout className={styles.layout}>
      <nav className={styles.navigation} aria-label={t("Settings topics")}>
        {(
          [
            ["general", "/setting/general", t("General")],
            ["editor", "/setting/editor", t("Editor")],
            ["connections", "/setting/connection", t("Connections")],
          ] as const
        ).map(([topic, to, label]) => (
          <Link
            key={topic}
            to={to}
            className={styles.link}
            aria-current={settingsTopic === topic ? "page" : undefined}
          >
            {label}
          </Link>
        ))}
      </nav>
      <div className={styles.content}>{children}</div>
    </ResourceLayout>
  );
}
