import { useRef, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { create } from "@bufbuild/protobuf";
import { Button } from "@lesomnus/cxz-ui";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import { SessionSearch } from "./session-search";
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
    timeMs: BigInt(Date.now()),
  }),
);
const meta = {
  title: "Conversation/SessionSearch",
  component: SessionSearch,
  args: {
    events,
    area: { current: null },
    jump: () => {},
    restoreFocus: () => {},
  },
  parameters: {
    docs: {
      description: {
        component:
          "Ctrl+Shift+F (Cmd+Shift+F on macOS) opens a persistent nonmodal search near the top of the current conversation. Typing immediately searches loaded conversation data, including virtualized messages outside the viewport. No Search API, pagination or history loading is used. The transcript highlights rendered matches without changing layout. Enter visits each occurrence from newest to oldest, then wraps; arrow keys select results and clicking jumps without dismissing search. Only Close or Escape dismiss it. This story supplies local messages; try 'network'.",
      },
    },
  },
  render: function Preview() {
    const area = useRef<HTMLDivElement>(null),
      input = useRef<HTMLTextAreaElement>(null);
    const [selected, setSelected] = useState("");
    const [loaded, setLoaded] = useState(events);
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
          className="transcript"
          style={{
            position: "absolute",
            top: 80,
            bottom: 80,
            insetInline: 16,
            overflow: "auto",
          }}
        >
          <div
            className="virtual-messages"
            style={{ position: "relative", paddingBottom: 500 }}
          >
            {loaded.map(({ text }, index) => (
              <div key={index} data-row={index + 1}>
                {index ? (
                  <article className="response">
                    <Markdown text={text} />
                  </article>
                ) : (
                  <article className="input">
                    <p className="message-body">{text}</p>
                  </article>
                )}
              </div>
            ))}
          </div>
        </div>
        <Button
          style={{ position: "absolute", right: 16, bottom: 96 }}
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
          events={loaded}
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
