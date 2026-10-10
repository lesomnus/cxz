import { useEffect, useState } from "react";
import type { SessionEvent } from "../../../../gen/cxz/session_pb";
import type { LoadEventDetails } from "../model/session-history";
import { DetailTabs, type DetailSection } from "./detail-tabs";
import { detail } from "../model/journal";
import { t } from "../../../shared/i18n/i18n";
import { useLocale } from "../../../shared/i18n/i18n-react";

// Mounting a preview triggers the read. Closing/replacing it cancels the request;
// native tool output never occupies the main transcript's historical cache.
export function useEventDetails(event: SessionEvent, load?: LoadEventDetails) {
  const [state, setState] = useState<{
    events?: SessionEvent[];
    error?: string;
  }>({});
  useEffect(() => {
    if (!load) return;
    const controller = new AbortController();
    setState({});
    void load(event.seq, controller.signal)
      .then((events) => {
        if (!controller.signal.aborted) setState({ events });
      })
      .catch((error) => {
        if (!controller.signal.aborted) setState({ error: String(error) });
      });
    return () => controller.abort();
  }, [event.seq, load]);
  return !load ? { events: [event] } : state;
}
export function EventDetails({
  event,
  loadDetails,
}: {
  event: SessionEvent;
  loadDetails?: LoadEventDetails;
}) {
  useLocale();
  const state = useEventDetails(event, loadDetails);
  if (state.error) return <p role="alert">{state.error}</p>;
  if (!state.events) return <p className="muted">{t("Loading…")}</p>;
  const sections: DetailSection[] = [];
  for (const e of state.events) {
    if (e.text)
      sections.push({
        id: `output-${e.seq}`,
        label: t("Output"),
        value: e.text,
      });
    if (e.payload.length)
      sections.push({
        id: `result-${e.seq}`,
        label: t("Result"),
        value: detail(e),
        language: "json",
      });
  }
  return <DetailTabs sections={sections} />;
}
