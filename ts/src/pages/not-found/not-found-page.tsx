import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";

export function NotFoundPage() {
  useLocale();
  return (
    <main className="empty route-not-found">
      <h1>{t("Page not found")}</h1>
      <RouteLink to="/sessions">{t("Back to sessions")}</RouteLink>
    </main>
  );
}
