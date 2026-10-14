import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { BadgedIcon } from "./badged-icon";
import { Button } from "../button/button";
import styles from "./badged-icon.stories.module.css";

function ChatIcon() {
  return (
    <svg
      width="64"
      height="64"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M5 4h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H9l-6 3V6a2 2 0 0 1 2-2Z" />
    </svg>
  );
}
const badge = <span className={styles.counter}>7</span>;
const meta = {
  title: "Components/BadgedIcon",
  component: BadgedIcon,
  tags: ["autodocs"],
  args: { children: <ChatIcon />, badge },
  decorators: [
    (Story) => (
      <div className={styles.backdrop}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          "A portable icon/badge wrapper. The icon child is unchanged; its own layer uses an SVG luminance mask that tracks the badge's real position and size. The surrounding clearance is transparent, so patterns, images and gradients show through. The default gap follows the SVG's rendered stroke width; gap can override it. Ellipse and rounded-rectangle masks are supported. Position, badgeStyle and badgeClassName move the badge without editing the icon. Only moving CSS animations request continuous frames; idle instances do not poll.",
      },
    },
  },
} satisfies Meta<typeof BadgedIcon>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const NoBadge: Story = { args: { badge: undefined } };
export const RoundedRectangle: Story = {
  args: {
    shape: "rounded-rectangle",
    badge: <span className={styles.pill}>99+</span>,
  },
};
export const MovingBadge: Story = {
  render: function Moving() {
    const [x, setX] = useState(85),
      [y, setY] = useState(20);
    const [visible, setVisible] = useState(true);
    return (
      <div className={styles.preview}>
        <BadgedIcon
          badge={visible ? badge : undefined}
          badgeStyle={{ left: `${x}%`, top: `${y}%` }}
          data-testid="moving-icon"
        >
          <ChatIcon />
        </BadgedIcon>
        <label>
          Badge X
          <input
            aria-label="Badge X"
            type="range"
            min="0"
            max="100"
            value={x}
            onChange={(event) => setX(Number(event.target.value))}
          />
        </label>
        <label>
          Badge Y
          <input
            aria-label="Badge Y"
            type="range"
            min="0"
            max="100"
            value={y}
            onChange={(event) => setY(Number(event.target.value))}
          />
        </label>
        <Button
          onClick={() => {
            setX(25);
            setY(75);
          }}
        >
          Move badge
        </Button>
        <Button onClick={() => setVisible((value) => !value)}>
          Toggle badge
        </Button>
      </div>
    );
  },
};
export const CssMotion: Story = { args: { badgeClassName: styles.moving } };
export const TransparentClearance: Story = {
  args: {
    children: (
      <svg width="64" height="64" viewBox="0 0 64 64" aria-hidden="true">
        <rect width="64" height="64" fill="#fff" />
      </svg>
    ),
    badge: <span className={styles.solidBadge} />,
    badgeStyle: { left: 36, top: 20 },
    gap: 4,
  },
};
