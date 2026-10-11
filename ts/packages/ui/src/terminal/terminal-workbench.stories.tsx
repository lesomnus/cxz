import {
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@lesomnus/cxz-ui";
import {
  TerminalWorkbench,
  type TerminalSurfaceProps,
} from "@lesomnus/cxz-ui/terminal";

// A deliberately simple renderer: no cxz providers, transport, xterm or app CSS.
// The session stays mounted when its existing DOM host moves to another pane.
function PreviewSession({
  id,
  groupId,
  active,
  visible,
  focused,
  placement,
  getHost,
  onStatus,
  onFocus,
  ref,
}: TerminalSurfaceProps) {
  const [host] = useState(() => document.createElement("div"));
  const [draft, setDraft] = useState("");
  const [lines, setLines] = useState<string[]>([]);
  const input = useRef<HTMLInputElement>(null);
  useImperativeHandle(
    ref,
    () => ({
      focus: () => input.current?.focus(),
      reconnect: () => {
        setLines([]);
        setDraft("");
      },
    }),
    [],
  );
  useEffect(
    () => onStatus(id, { state: "", ended: false, clipboardNotice: "" }),
    [id, onStatus],
  );
  useLayoutEffect(() => {
    const slot = getHost(groupId);
    if (slot && host.parentElement !== slot) slot.append(host);
    host.hidden = !active;
    if (visible && active && focused) input.current?.focus();
  }, [host, groupId, active, visible, focused, placement, getHost]);
  useLayoutEffect(() => () => host.remove(), [host]);
  return createPortal(
    <div
      style={{ padding: 8, fontFamily: "monospace" }}
      onFocusCapture={() => onFocus(id)}
    >
      <p>Renderer preview · {id} · no server connection</p>
      <pre>{lines.join("\n")}</pre>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          setLines((current) => [...current, draft]);
          setDraft("");
        }}
      >
        <input
          ref={input}
          aria-label={`Input ${id}`}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          style={{
            width: "100%",
            boxSizing: "border-box",
            background: "var(--ui-shade-27)",
            color: "inherit",
            border: 0,
            padding: 6,
            font: "inherit",
          }}
        />
      </form>
    </div>,
    host,
  );
}

const meta = {
  title: "Components/TerminalWorkbench",
  component: TerminalWorkbench,
  args: {
    visible: true,
    hide: () => {},
    renderSession: (props) => <PreviewSession {...props} />,
  },
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Reusable terminal tabs and docking panes with an injected session renderer. Add follows the last tab; drag tabs to reorder, merge or split. The renderer owns its connection, buffer and focus handle. This preview uses plain React inputs and needs neither cxz nor xterm.",
      },
    },
  },
  render: function Example(args) {
    const [visible, setVisible] = useState(true);
    return (
      <div style={{ height: "85vh", display: "flex", flexDirection: "column" }}>
        <div style={{ flex: 1, padding: 16 }}>
          <Button onClick={() => setVisible((current) => !current)}>
            Toggle terminal
          </Button>
          <p>
            Type a draft, create another tab, and drag a tab to a pane edge.
            Drafts and submitted lines survive docking.
          </p>
        </div>
        <TerminalWorkbench
          {...args}
          visible={visible}
          hide={() => setVisible(false)}
        />
      </div>
    );
  },
} satisfies Meta<typeof TerminalWorkbench>;
export default meta;
export const Portable: StoryObj<typeof meta> = {};
