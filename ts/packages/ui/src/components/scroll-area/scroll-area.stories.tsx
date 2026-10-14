import { useRef, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button, ScrollArea } from "@lesomnus/cxz-ui";

const entries = (count: number) =>
  Array.from({ length: count }, (_, i) => (
    <div key={i} style={{ minHeight: 40, padding: "var(--ui-space)" }}>
      Entry {i + 1}
    </div>
  ));
const meta = {
  title: "Components/ScrollArea",
  component: ScrollArea,
  tags: ["autodocs"],
  args: {
    scrollbarLabel: "Scroll entries",
    style: { height: 320 },
    children: entries(60),
  },
  parameters: {
    docs: {
      description: {
        component:
          "A standalone vertical scroll viewport with native wheel/touch scrolling, a centered rail that grows with overflow, capped handles, hover/focus/drag visibility, proximity expansion and background-colored edge fades. Its CSS Modules and --ui-scroll-area-* tokens work without cxz app styles or a provider. Supply a bounded height (or use a constrained flex parent), arbitrary children and an accessible scrollbarLabel. viewportRef exposes the native scroll element; onScroll reports its events. className, viewportClassName, contentClassName and classNames customize slots. This component does not virtualize or fetch its content.",
      },
    },
  },
} satisfies Meta<typeof ScrollArea>;
export default meta;
export const LongList: StoryObj<typeof meta> = {};
export const ShortOverflow: StoryObj<typeof meta> = {
  args: { children: entries(9) },
};
export const NoOverflow: StoryObj<typeof meta> = {
  args: { children: entries(3) },
};
export const GrowingContent: StoryObj<typeof meta> = {
  render: function Growing(args) {
    const [count, setCount] = useState(9);
    const [top, setTop] = useState(0);
    const viewport = useRef<HTMLDivElement>(null);
    return (
      <section>
        <header
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "var(--ui-dialog-padding)",
            background: "var(--ui-shade-16)",
          }}
        >
          <strong>Entries</strong>
          <Button onClick={() => setCount((old) => old + 12)}>
            Add entries
          </Button>
        </header>
        <ScrollArea
          {...args}
          viewportRef={viewport}
          onScroll={(event) => {
            args.onScroll?.(event);
            setTop(Math.round(event.currentTarget.scrollTop));
          }}
          style={{ ...args.style, borderRadius: "var(--ui-dialog-radius)" }}
        >
          {entries(count)}
        </ScrollArea>
        <Button
          onClick={() => {
            if (viewport.current)
              viewport.current.scrollTop = viewport.current.scrollHeight;
          }}
        >
          Scroll to bottom
        </Button>
        <output aria-label="Scroll position">{top}</output>
      </section>
    );
  },
};
