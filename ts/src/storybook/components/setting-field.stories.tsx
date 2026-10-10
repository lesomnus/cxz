import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SettingField } from "@lesomnus/cxz-ui";
import { SegmentedControl } from "@lesomnus/cxz-ui";

function ThemeControl() {
  const [value, setValue] = useState("dark");
  return (
    <SegmentedControl
      label="Appearance"
      value={value}
      options={[
        { value: "light", label: "Light" },
        { value: "dark", label: "Dark" },
        { value: "system", label: "System" },
      ]}
      onChange={setValue}
    />
  );
}
const meta = {
  title: "Settings/SettingField",
  component: SettingField,
  tags: ["autodocs"],
  args: {
    title: "Appearance",
    settingId: "ui.theme",
    summary: "Choose Light, Dark, or follow your system appearance.",
    children: <ThemeControl />,
  },
  decorators: [
    (Story) => (
      <div
        className="settings-page"
        style={{
          width: "min(var(--settings-column-width), calc(100vw - 48px))",
        }}
      >
        <section className="settings-group">
          <Story />
        </section>
      </div>
    ),
  ],
  render: (args) => (
    <SettingField {...args}>
      <ThemeControl />
    </SettingField>
  ),
} satisfies Meta<typeof SettingField>;
export default meta;
type Story = StoryObj<typeof meta>;
export const SummaryOnly: Story = {};
export const WithDetails: Story = {
  args: {
    details:
      "System appearance follows the operating system. Your conversation content and code are preserved.",
  },
};
