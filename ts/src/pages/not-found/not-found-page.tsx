import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { useWorkspaceRoute } from "#src/app/router.tsx";

export function NotFoundPage() {
  useLocale();
  const { connection } = useWorkspaceRoute();
  return (
    <main className="empty route-not-found">
      <h1>{t("Page not found")}</h1>
      {connection && <p>{t("This connection is unavailable.")}</p>}
      <RouteLink to="/">{t("Back to sessions")}</RouteLink>
    </main>
  );
}
