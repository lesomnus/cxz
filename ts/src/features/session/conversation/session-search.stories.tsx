import { eventListSource } from "#src/features/session/model/event-source.ts";
import { useRef, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { create } from "@bufbuild/protobuf";
import { Button } from "@lesomnus/nox";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import { SessionSearch } from "./session-search";
import { Transcript } from "./transcript";
import { Markdown } from "#src/shared/content/markdown.tsx";

const messages = [
  "Investigate network latency in the terminal.",
  "The **network** transfer now uses bounded batches.\n\n" +
    "Transfer diagnostics.\n\n".repeat(12) +
    "Second network match in this reply.",
  "Update the file preview layout.",
];
const events = messages.map((text, index) =>
  create(SessionEventSchema, {
    seq: BigInt(index + 1),
    kind: index ? "assistant" : "input",
    text,
    timeMs: BigInt(Date.now() - (messages.length - index) * 5 * 60_000),
  }),
);
const meta = {
  title: "Conversation/SessionSearch",
  component: SessionSearch,
  args: {
    source: eventListSource(events),
    area: { current: null },
    jump: () => {},
    restoreFocus: () => {},
  },
  parameters: {
    docs: {
      description: {
        component:
          "Ctrl+Shift+F (Cmd+Shift+F on macOS) opens a persistent nonmodal search near the top of the current conversation. Typing immediately searches loaded conversation data, including virtualized messages outside the viewport. No Search API, pagination or history loading is used. Match case, Match whole word and Use regular expression toggles apply the same compiled expression to result previews, transcript highlights, scroll markers and navigation. The default is case-insensitive literal matching; whole-word boundaries recognize Unicode letters, numbers and combining marks. Invalid regular expressions show a field error, and empty matches are omitted. The transcript highlights rendered matches without changing layout and adds green search ticks beside its scroll handle, sharing the existing elastic marker motion. Enter visits each occurrence from newest to oldest, then wraps; arrow keys select results and clicking jumps without dismissing search. Only Close or Escape dismiss it. This story supplies local messages; try 'network'.",
      },
    },
  },
  render: function Preview({ source }) {
    const initialEvents = source();
    const area = useRef<HTMLDivElement>(null),
      input = useRef<HTMLTextAreaElement>(null);
    const [selected, setSelected] = useState("");
    const [loaded, setLoaded] = useState(initialEvents);
    const pane = useRef<HTMLDivElement>(null),
      follow = useRef(false);
    const [searchMatches, setSearchMatches] = useState<readonly string[]>([]);
    return (
      <div
        ref={area}
        style={{
          position: "relative",
          width: "min(640px, 90vw)",
          height: 680,
          background: "var(--background)",
          padding: "var(--ui-dialog-padding)",
        }}
      >
        <Button
          onClick={() => {
            input.current?.focus();
            input.current?.dispatchEvent(
              new KeyboardEvent("keydown", {
                key: "f",
                ctrlKey: true,
                shiftKey: true,
                bubbles: true,
                cancelable: true,
              }),
            );
          }}
        >
          Search conversation
        </Button>
        <p>
          Search is scoped to this conversation. Clicking this text does not
          close it.
        </p>
        {selected && <p>Jumped to message {selected}</p>}
        <div
          style={{
            position: "absolute",
            top: 80,
            bottom: 80,
            insetInline: 16,
            display: "flex",
            minHeight: 0,
          }}
        >
          <Transcript
            pane={pane}
            source={eventListSource(loaded)}
            follow={follow}
            searchMatches={searchMatches}
            notice={null}
            onScroll={() => {}}
            onNavigate={() => {}}
            onTension={() => {}}
            onMotion={() => {}}
            onReadingMove={() => {}}
            precedingPrompt={undefined}
            jumpTarget={undefined}
            onPromptJump={setSelected}
            onJumped={() => {}}
            older={() => {}}
            newer={() => {}}
            render={(event) =>
              event.kind === "input" ? (
                <article className="input">
                  <p className="message-body">{event.text}</p>
                </article>
              ) : (
                <article className="response">
                  <Markdown text={event.text} />
                </article>
              )
            }
          />
        </div>
        <Button
          style={{ position: "absolute", right: 16, bottom: 96, zIndex: 4 }}
          onClick={() =>
            setLoaded((old) => [
              ...old,
              create(SessionEventSchema, {
                seq: BigInt(old.length + 1),
                kind: "assistant",
                text: "Another network response.",
                timeMs: BigInt(Date.now()),
              }),
            ])
          }
        >
          Append response
        </Button>
        <textarea
          ref={input}
          aria-label="Conversation input"
          style={{
            position: "absolute",
            bottom: "var(--ui-dialog-padding)",
            insetInline: "var(--ui-dialog-padding)",
            background: "var(--ui-shade-17)",
            color: "inherit",
            border: 0,
          }}
        />
        <SessionSearch
          source={eventListSource(loaded)}
          onMatchesChange={setSearchMatches}
          area={area}
          jump={setSelected}
          restoreFocus={() => input.current?.focus()}
        />
      </div>
    );
  },
} satisfies Meta<typeof SessionSearch>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
export const WithResults: StoryObj<typeof meta> = {
  parameters: {
    docs: {
      description: {
        story:
          "Opens with 'network' already entered and matching conversation results visible. Inspect the search bar, selected and unselected results, transcript highlights and scrollbar markers without setting up a search. The real search remains interactive: edit the query, navigate results, append a response or close with Escape.",
      },
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Search conversation" }),
    );
    const input = await canvas.findByRole("searchbox", {
      name: "Search this conversation",
    });
    await userEvent.clear(input);
    await userEvent.type(input, "network");
  },
};

export const WithSearchOptions: StoryObj<typeof meta> = {
  args: {
    source: eventListSource([
      create(SessionEventSchema, {
        seq: 1n,
        kind: "input",
        text: "**Network** and NETWORK differ from network and networking.",
        timeMs: BigInt(Date.now() - 10 * 60_000),
      }),
      create(SessionEventSchema, {
        seq: 2n,
        kind: "assistant",
        text: "한글 검색: 한글, 한글자, 새한글, (한글).\n\nUse network or net; not networking.",
        timeMs: BigInt(Date.now() - 5 * 60_000),
      }),
    ]),
  },
  parameters: {
    docs: {
      description: {
        story:
          "All three search modes enabled. The expression net(?:work)? finds lowercase standalone 'net' and 'network', excluding Network, NETWORK and networking. Try '한글' for Unicode whole-word matching, or '[' to inspect the invalid-pattern state.",
      },
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Search conversation" }),
    );
    const input = await canvas.findByRole("searchbox", {
      name: "Search this conversation",
    });
    await userEvent.type(input, "net(?:work)?");
    for (const name of [
      "Match case",
      "Match whole word",
      "Use regular expression",
    ]) {
      await userEvent.click(canvas.getByRole("button", { name }));
    }
    await userEvent.click(input);
  },
};
