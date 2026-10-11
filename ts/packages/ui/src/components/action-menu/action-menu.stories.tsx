import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ActionMenu } from "@lesomnus/cxz-ui";

const meta = {
  title: "Components/ActionMenu",
  component: ActionMenu,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "A menu beside its trigger, used for More actions and checked choices. Defaults to an ellipsis button; triggerContent can supply text or an icon. This is separate from ValueMenu, the combobox/listbox that overlays its current value onto the trigger position.",
      },
    },
  },
  args: {
    label: "More",
    items: [
      { label: "Details", run: () => {} },
      { label: "Restart", run: () => {} },
      { label: "Stop", danger: true, run: () => {} },
    ],
  },
} satisfies Meta<typeof ActionMenu>;
export default meta;
type Story = StoryObj<typeof meta>;

export const More: Story = {};
export const Language: Story = {
  args: { label: "Voice language" },
  render: function LanguageMenu(args) {
    const [language, setLanguage] = useState("English (US)");
    return (
      <ActionMenu
        {...args}
        triggerContent={language}
        items={["English (US)", "한국어", "日本語"].map((label) => ({
          label,
          checked: label === language,
          run: () => setLanguage(label),
        }))}
      />
    );
  },
  decorators: [
    (Story) => (
      <div style={{ width: 180 }}>
        <Story />
      </div>
    ),
  ],
};
export const Disabled: Story = {
  args: { disabled: true, disabledReason: "Unavailable while working" },
};
