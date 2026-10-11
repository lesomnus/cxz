import { Markdown } from "#src/shared/content/markdown.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import type { TurnSummary } from "#src/features/session/model/session-aux.ts";

// A summary is labelled and set apart from the response above it, because it is
// what a model wrote about the turn rather than what the agent answered. The
// label is not decoration: an unlabelled paragraph in a conversation reads as
// something somebody said.
export function TurnSummaryView({ summary }: { summary: TurnSummary }) {
  useLocale();
  return (
    <aside
      className={`turn-summary${summary.failed ? " error" : ""}`}
      aria-label={t("Summary")}
    >
      <small className="turn-summary-label">{t("Summary")}</small>
      {summary.loading ? (
        <p className="muted">{t("Summarizing…")}</p>
      ) : (
        <Markdown text={summary.text} />
      )}
    </aside>
  );
}
