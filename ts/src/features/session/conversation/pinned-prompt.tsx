import { t } from "../../../shared/i18n/i18n";
import { useLocale } from "../../../shared/i18n/i18n-react";
import { useEffect, useRef, useState } from "react";
import type { SessionEvent } from "../../../../gen/cxz/session_pb";
import { Button } from "@lesomnus/cxz-ui";
import { InputMessage } from "./input-message";

export function PinnedPrompt({
  prompt,
  peek,
  height,
  jump,
}: {
  prompt: SessionEvent;
  peek: number;
  height: number;
  jump: (seq: string) => void;
}) {
  useLocale();
  const root = useRef<HTMLDivElement>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const hold = useRef({
    openedAt: 0,
    expanded: false,
    pointer: false,
    focus: false,
  });
  const [expanded, setExpanded] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const distanceCleared = useRef(false);
  useEffect(() => () => clearTimeout(timer.current), []);
  // A click suppresses the old overlay while an uncached input is fetched.
  // Once that input is at the upper edge, ordinary distance gating takes over.
  useEffect(() => {
    if (!dismissed) return;
    if (peek === 0) distanceCleared.current = true;
    else if (distanceCleared.current) {
      distanceCleared.current = false;
      setDismissed(false);
    }
  }, [peek, dismissed]);

  function enter(kind: "pointer" | "focus") {
    if (dismissed || (!hold.current.expanded && peek === 0)) return;
    clearTimeout(timer.current);
    hold.current[kind] = true;
    if (hold.current.expanded) return;
    hold.current.expanded = true;
    hold.current.openedAt = performance.now();
    setExpanded(true);
  }
  function leave(kind: "pointer" | "focus") {
    const current = hold.current;
    current[kind] = false;
    if (!current.expanded || current.pointer || current.focus) return;
    clearTimeout(timer.current);
    const now = performance.now();
    timer.current = setTimeout(
      () => {
        if (hold.current.pointer || hold.current.focus) return;
        hold.current.expanded = false;
        setExpanded(false);
      },
      Math.max(current.openedAt + 3000, now + 1000) - now,
    );
  }
  const available = !dismissed && (expanded || peek > 0);
  return (
    <div
      className="pinned-prompt"
      ref={root}
      data-pinned-seq={prompt.seq.toString()}
      data-available={available}
      data-expanded={expanded}
      data-dismissed={dismissed}
      inert={!available}
      aria-hidden={!available}
      style={
        {
          "--prompt-peek": `${dismissed ? 0 : 8 * peek}px`,
          "--prompt-height": `${height}px`,
          opacity: dismissed ? 0 : expanded ? 1 : peek,
        } as React.CSSProperties
      }
      onPointerEnter={() => enter("pointer")}
      onPointerLeave={() => leave("pointer")}
      onFocus={() => enter("focus")}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) leave("focus");
      }}
    >
      <div
        className="pinned-prompt-proximity"
        aria-hidden="true"
        onPointerDown={() =>
          root.current?.querySelector("button")?.focus({ preventScroll: true })
        }
      />
      <Button
        type="button"
        aria-label={t("Jump to user message")}
        onClick={(event) => {
          clearTimeout(timer.current);
          hold.current = {
            openedAt: 0,
            expanded: false,
            pointer: false,
            focus: false,
          };
          setExpanded(false);
          distanceCleared.current = false;
          setDismissed(true);
          event.currentTarget.blur();
          jump(prompt.seq.toString());
        }}
      >
        <InputMessage event={prompt} />
      </Button>
    </div>
  );
}
