import { createRoot } from "react-dom/client";
import { UIProvider, Button } from "@lesomnus/nox";
import { useState } from "react";
import { MAX_EVENTS } from "#src/features/session/model/journal.ts";
import { ComponentPreview } from "#src/storybook/conversation-preview.tsx";
import { storyEvent, responseEvent } from "#src/storybook/fixtures.ts";
import "#src/app/styles/style.css";
import { installReactPerformanceBudget } from "#src/shared/browser/react-performance.ts";
import { ThemeProvider } from "#src/shared/theme/theme.tsx";
if (new URL(location.href).searchParams.get("budget") !== "off")
  installReactPerformanceBudget();

function Fixture() {
  const [events, setEvents] = useState(() =>
    Array.from(
      {
        length: Math.min(
          MAX_EVENTS,
          Math.max(
            1,
            Number(new URL(location.href).searchParams.get("count")) || 1000,
          ),
        ),
      },
      (_, index) =>
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
    ),
  );
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
createRoot(document.getElementById("root")!).render(
  <ThemeProvider>
    <Fixture />
  </ThemeProvider>,
);
