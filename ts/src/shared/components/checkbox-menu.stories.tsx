import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { CheckboxMenu } from "./checkbox-menu";
function Preview({ disabledLast = false }: { disabledLast?: boolean }) {
  const [value, setValue] = useState(["info", "error"]);
  return (
    <div style={{ display: "grid", gap: "var(--ui-space)" }}>
      <CheckboxMenu
        label="Log level"
        allLabel="All levels"
        options={[
          { value: "info", label: "Information" },
          { value: "warn", label: "Warnings" },
          { value: "error", label: "Errors", disabled: disabledLast },
        ]}
        value={value}
        choose={setValue}
      />
      <output aria-label="Selected values">{value.join(", ") || "None"}</output>
    </div>
  );
}
const meta = {
  title: "Components/CheckboxMenu",
  component: Preview,
  parameters: {
    docs: {
      description: {
        component:
          "A reusable cxz multi-select dropdown built from Nox MenuSurface, MenuItem and MenuScrollArea through AnchoredMenu. Toggling keeps the menu open. Arrow keys/Home/End move through entries; Space/Enter toggle, and Escape/outside dismiss. The All row supports mixed state and preserves disabled selections. Trigger text stays fixed while filters change. Placement, glass, corners and entrance animation use the shared menu implementation.",
      },
    },
  },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const DisabledOption: Story = { args: { disabledLast: true } };
