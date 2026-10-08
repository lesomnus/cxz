import { useEffect, useState } from "react";
import type { SessionEvent } from "../gen/cxz/session_pb";
import type { LoadEventDetails } from "./session-history";
import { detail } from "./journal";
import { t } from "./i18n";
import { useLocale } from "./i18n-react";

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
  return (
    <>
      {state.events.map((e) => (
        <div key={e.seq.toString()}>
          <pre>{e.text}</pre>
          {e.payload.length > 0 && <pre>{detail(e)}</pre>}
        </div>
      ))}
    </>
  );
}
