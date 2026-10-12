import type { SessionEvent } from "#gen/cxz/session_pb";
import type { LoadEventDetails } from "#src/features/session/model/session-history.ts";
import { eventDiagnostic } from "#src/features/session/model/event-diagnostic.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { ActivityCard } from "./activity-card";
import { EventDetails, useEventDetails } from "./event-details";
import styles from "./activity-card.module.css";

export function DiagnosticCard({
  event,
  loadDetails,
}: {
  event: SessionEvent;
  loadDetails?: LoadEventDetails;
}) {
  useLocale();
  // Older Transcript servers omit payloads, leaving only a generic event label.
  // Fetch those errors once on mount; current summaries already carry a message.
  const missingMessage =
    !event.payload.length &&
    /^(?:(?:codex|claude) )?(?:error|request failed)$/i.test(event.text.trim());
  const state = useEventDetails(
    event,
    missingMessage ? loadDetails : undefined,
  );
  const source = state.events?.find((item) => item.seq === event.seq) ?? event;
  const diagnostic = eventDiagnostic(source) ?? eventDiagnostic(event)!;
  return (
    <ActivityCard
      seq={event.seq}
      timeMs={event.timeMs}
      tone="error"
      title={t("Error")}
      details={() => (
        <EventDetails
          event={source}
          loadDetails={source === event ? loadDetails : undefined}
        />
      )}
    >
      <span className={styles.diagnostic}>
        <span>
          {t("Error")} · {diagnostic.message || t("Request failed")}
        </span>
        {(diagnostic.code || diagnostic.retrying !== undefined) && (
          <small className={styles.metadata}>
            {[
              diagnostic.code,
              diagnostic.retrying === true
                ? t("Retrying…")
                : diagnostic.retrying === false
                  ? t("No automatic retry")
                  : undefined,
            ]
              .filter(Boolean)
              .join(" · ")}
          </small>
        )}
      </span>
    </ActivityCard>
  );
}
