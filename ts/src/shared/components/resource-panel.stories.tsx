import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button, PanelItem } from "@lesomnus/nox";
import { ResourcePanel } from "./resource-panel";
import { AgentIdentity } from "./agent-identity";

const meta = {
  title: "Navigation/ResourcePanel",
  component: ResourcePanel,
  decorators: [
    (Story) => (
      <div style={{ width: 180, height: 440, display: "grid" }}>
        <Story />
      </div>
    ),
  ],
  args: {
    title: "Sessions",
    actions: <Button aria-label="Add session">+</Button>,
    children: Array.from({ length: 16 }, (_, i) => (
      <PanelItem key={i} aria-pressed={i === 0} pressTarget=".session-heading">
        <AgentIdentity
          agent={i % 2 ? "claude" : "codex"}
          name={`Conversation ${i + 1}`}
          alias={`session-${i + 1}`}
        />
      </PanelItem>
    )),
  },
  parameters: {
    docs: {
      description: {
        component:
          "Persistent navigation with a fixed header and rounded scrollable body. Body side insets match the header action's control inset, including the panel's continuous right edge. Shares DialogCard's body color, radius and padding tokens. Sessions, accounts, projects and settings use this frame; its action can open an AnchoredDialog beside the panel.",
      },
    },
  },
} satisfies Meta<typeof ResourcePanel>;
export default meta;
export const Sessions: StoryObj<typeof meta> = {};
export const Settings: StoryObj<typeof meta> = {
  args: {
    title: "Settings",
    actions: undefined,
    children: (
      <>
        <PanelItem aria-pressed="true">General</PanelItem>
        <PanelItem>Editor</PanelItem>
      </>
    ),
  },
};
