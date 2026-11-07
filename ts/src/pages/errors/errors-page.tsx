import { useNavigate } from "@tanstack/react-router";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { ResourcePage } from "#src/shared/components/resource-layout.tsx";
import { ErrorPanel } from "#src/shared/errors/error-panel.tsx";
import {
  clientErrors,
  type ClientErrors,
} from "#src/shared/errors/client-errors.ts";
import { resourceLocation } from "#src/shared/navigation/resource-route.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./errors-page.module.css";

export function ErrorsPage({
  store = clientErrors(),
}: {
  store?: ClientErrors;
}) {
  useLocale();
  const { errorTab } = useWorkspaceRoute();
  const navigate = useNavigate();
  return (
    <ResourcePage aria-label={t("Errors")} className={styles.page}>
      <ErrorPanel
        store={store}
        mode="page"
        tab={errorTab ?? "active"}
        onTabChange={(tab) =>
          void navigate({
            ...resourceLocation("errors"),
            search: { tab: tab === "history" ? "history" : undefined },
            replace: true,
          })
        }
      />
    </ResourcePage>
  );
}
