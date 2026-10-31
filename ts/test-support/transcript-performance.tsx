import { createRoot } from "react-dom/client";
import { UIProvider, Button } from "@lesomnus/nox";
import { useCallback, useEffect, useRef, useState } from "react";
import { Transcript } from "#src/features/session/conversation/transcript.tsx";
import { VirtualMessages } from "#src/features/session/conversation/virtual-messages.tsx";
import { eventListSource } from "#src/features/session/model/event-source.ts";
import type { SessionEvent } from "#gen/cxz/session_pb";
import { MAX_EVENTS } from "#src/features/session/model/journal.ts";
import { ComponentPreview } from "#src/storybook/conversation-preview.tsx";
import {
  storyEvent,
  responseEvent,
  mixedConversationEvents,
  complexConversationEvents,
} from "#src/storybook/fixtures.ts";
import "#src/app/styles/style.css";
import { installReactPerformanceBudget } from "#src/shared/browser/react-performance.ts";
import { ThemeProvider } from "#src/shared/theme/theme.tsx";
if (new URL(location.href).searchParams.get("budget") !== "off")
  installReactPerformanceBudget();

function Fixture() {
  const [events, setEvents] = useState(() => {
    const params = new URL(location.href).searchParams;
    const count = Math.min(
      MAX_EVENTS,
      Math.max(1, Number(params.get("count")) || 1000),
    );
    if (["mixed", "complex"].includes(params.get("content") ?? ""))
      return (
        params.get("content") === "complex"
          ? complexConversationEvents
          : mixedConversationEvents
      )(Math.ceil((count * 3) / 7)).slice(0, count);
    return Array.from({ length: count }, (_, index) =>
      index % 2 === 0
        ? storyEvent(
            index + 1,
            "input",
            `Input ${index}\n` +
              "A reasonably long conversation message.\n".repeat(3),
          )
        : responseEvent(
            index + 1,
            "codex",
            `Response ${index}\n\n` +
              "A reasonably long conversation message.\n".repeat(3),
          ),
    );
  });
  return (
    <UIProvider>
      <Button
        onClick={() =>
          setEvents((previous) => [
            ...previous,
            ...Array.from({ length: 10 }, (_, index) =>
              responseEvent(
                previous.length + index + 1,
                "codex",
                "Additional history loaded.",
              ),
            ),
          ])
        }
      >
        Load more
      </Button>
      <ComponentPreview events={events} />
    </UIProvider>
  );
}

const noop = () => {};
// Start at the edge of a loaded window with more history available. A held
// outward pull must request that page even before native scrollTop can change.
function HistoryEdgeFixture() {
  const pane = useRef<HTMLDivElement>(null);
  const follow = useRef(false);
  const requested = useRef(false);
  const timer = useRef<number | undefined>(undefined);
  const [loads, setLoads] = useState(0);
  const [events, setEvents] = useState(() =>
    Array.from({ length: 100 }, (_, index) =>
      storyEvent(1000 + index, "assistant", `Row ${1000 + index}`),
    ),
  );
  useEffect(() => () => clearTimeout(timer.current), []);
  const older = useCallback(() => {
    if (requested.current) return;
    requested.current = true;
    setLoads(1);
    timer.current = window.setTimeout(
      () =>
        setEvents((previous) => [
          ...Array.from({ length: 20 }, (_, index) =>
            storyEvent(980 + index, "assistant", `Row ${980 + index}`),
          ),
          ...previous,
        ]),
      100,
    );
  }, []);
  const render = useCallback(
    (event: SessionEvent) => (
      <div data-read-seq={event.seq.toString()} style={{ height: 80 }}>
        {event.text}
      </div>
    ),
    [],
  );
  return (
    <>
      <output aria-label="History loads">{loads}</output>
      <div className="conversation" style={{ height: 760 }}>
        <Transcript
          pane={pane}
          source={eventListSource(events)}
          follow={follow}
          render={render}
          notice={null}
          onNavigate={noop}
          onScroll={noop}
          onTension={noop}
          onMotion={noop}
          onReadingMove={noop}
          precedingPrompt={undefined}
          jumpTarget={undefined}
          onPromptJump={noop}
          onJumped={noop}
          older={older}
          newer={noop}
        />
      </div>
    </>
  );
}
// A small renderer probe isolates row reuse from EventView's own memoization.
function RowReuseFixture() {
  const pane = useRef<HTMLDivElement>(null);
  const follow = useRef(false);
  const calls = useRef<Record<string, number>>({});
  const [report, setReport] = useState("");
  const [events, setEvents] = useState(() =>
    Array.from({ length: 100 }, (_, index) =>
      storyEvent(index + 1, "assistant", `Row ${index + 1}`),
    ),
  );
  const render = useCallback((event: SessionEvent) => {
    const id = event.seq.toString();
    calls.current[id] = (calls.current[id] ?? 0) + 1;
    return (
      <div data-rendered-row={id} style={{ height: 80 }}>
        {event.text}
      </div>
    );
  }, []);
  return (
    <>
      <Button onClick={() => setReport(JSON.stringify(calls.current))}>
        Read render counts
      </Button>
      <Button
        onClick={() => {
          pane.current!.querySelector<HTMLElement>(
            "[data-rendered-row]",
          )!.style.height = "160px";
        }}
      >
        Grow first row
      </Button>
      <Button
        onClick={() =>
          setEvents((previous) => [
            { ...previous[0], text: "Updated row" },
            ...previous.slice(1),
          ])
        }
      >
        Replace first row
      </Button>
      <output aria-label="Render counts">{report}</output>
      <div
        ref={pane}
        className="transcript"
        style={{ height: 400, overflow: "auto", position: "relative" }}
      >
        <VirtualMessages
          pane={pane}
          follow={follow}
          source={eventListSource(events)}
          render={render}
          changed={noop}
          precedingPrompt={undefined}
          onPromptChange={noop}
        />
      </div>
    </>
  );
}
createRoot(document.getElementById("root")!).render(
  <ThemeProvider>
    {new URL(location.href).searchParams.get("probe") === "rows" ? (
      <UIProvider>
        <RowReuseFixture />
      </UIProvider>
    ) : new URL(location.href).searchParams.get("probe") === "history-edge" ? (
      <UIProvider>
        <HistoryEdgeFixture />
      </UIProvider>
    ) : (
      <Fixture />
    )}
  </ThemeProvider>,
);
