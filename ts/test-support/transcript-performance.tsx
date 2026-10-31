import { createRoot } from "react-dom/client";
import { UIProvider, Button } from "@lesomnus/nox";
import { useCallback, useRef, useState } from "react";
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
    ) : (
      <Fixture />
    )}
  </ThemeProvider>,
);
