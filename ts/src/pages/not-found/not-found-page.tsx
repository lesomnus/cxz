import { t } from "../../shared/i18n/i18n";
import { useLocale } from "../../shared/i18n/i18n-react";
import { RouteLink } from "../../shared/navigation/route-link";

export function NotFoundPage() {
  useLocale();
  return (
    <main className="empty route-not-found">
      <h1>{t("Page not found")}</h1>
      <RouteLink to="/sessions">{t("Back to sessions")}</RouteLink>
    </main>
  );
}
