import { useEffect, useId, useRef, useState } from "react";
import { Button } from "@lesomnus/cxz-ui";
import { ElapsedTime } from "@lesomnus/cxz-ui";
import { useLocale } from "../../../shared/i18n/i18n-react";
import { t } from "../../../shared/i18n/i18n";
import type { TurnProgress } from "../model/turn-progress";

export function TurnControls({
  turn,
  busy,
  interrupt,
}: {
  turn: TurnProgress;
  busy: boolean;
  interrupt: () => void;
}) {
  useLocale();
  const root = useRef<HTMLDivElement>(null);
  const latest = useRef({ turn, busy, interrupt });
  latest.current = { turn, busy, interrupt };
  const confirmation = useRef<{ key?: string; source?: string; until: number }>(
    { until: 0 },
  );
  const timeout = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [armed, setArmed] = useState(false);
  const hint = useId();
  function reset() {
    confirmation.current = { until: 0 };
    clearTimeout(timeout.current);
    setArmed(false);
  }
  function confirm(source: "click" | "escape") {
    const current = latest.current;
    if (!current.turn.active || current.busy) return;
    const now = performance.now();
    const previous = confirmation.current;
    if (
      previous.key === current.turn.key &&
      previous.source === source &&
      now < previous.until
    ) {
      reset();
      current.interrupt();
      return;
    }
    confirmation.current = { key: current.turn.key, source, until: now + 3000 };
    clearTimeout(timeout.current);
    setArmed(true);
    timeout.current = setTimeout(reset, 3000);
  }
  useEffect(() => {
    reset();
  }, [turn.key, turn.active, busy]);
  useEffect(() => {
    const escape = (event: KeyboardEvent) => {
      if (
        event.key !== "Escape" ||
        event.repeat ||
        event.isComposing ||
        event.defaultPrevented ||
        event.ctrlKey ||
        event.altKey ||
        event.metaKey ||
        event.shiftKey
      )
        return;
      const target = event.target as Node | null;
      const conversation = root.current?.closest(".conversation");
      if (
        target !== document.body &&
        target !== document.documentElement &&
        (!target || !conversation?.contains(target))
      )
        return;
      // Preview dismissal has priority even though this listener was registered
      // before the card opened and attached its own Escape listener.
      if (conversation?.querySelector('.floating-card[data-active="true"]'))
        return;
      if (!latest.current.turn.active || latest.current.busy) return;
      event.preventDefault();
      confirm("escape");
    };
    document.addEventListener("keydown", escape);
    window.addEventListener("blur", reset);
    return () => {
      document.removeEventListener("keydown", escape);
      window.removeEventListener("blur", reset);
      clearTimeout(timeout.current);
    };
  }, []);
  return (
    <div className="turn-controls" ref={root}>
      <span className="stop-control">
        <Button
          type="button"
          className="toolbar-button stop"
          data-armed={armed}
          aria-label={t("Stop response")}
          aria-keyshortcuts="Escape"
          aria-describedby={hint}
          disabled={!turn.active || busy}
          onClick={() => confirm("click")}
        >
          <svg
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="currentColor"
            aria-hidden="true"
          >
            <rect x="6" y="6" width="12" height="12" rx="1.5" />
          </svg>
        </Button>
        <span id={hint} className="send-shortcut" role="tooltip">
          {armed
            ? t("Repeat within 3 seconds to stop")
            : t("Click twice or press Esc twice within 3 seconds")}
        </span>
      </span>
      <ElapsedTime
        startedAt={turn.active ? turn.startedAt : undefined}
        label={t("Response elapsed time")}
      />
    </div>
  );
}
