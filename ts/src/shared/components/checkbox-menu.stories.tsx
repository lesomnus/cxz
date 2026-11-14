import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { CheckboxMenu } from "./checkbox-menu";
function Preview({ disabledLast = false }: { disabledLast?: boolean }) {
  const [value, setValue] = useState(["info", "error"]);
  const [applies, setApplies] = useState(0);
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
        choose={(next) => {
          setValue(next);
          setApplies((count) => count + 1);
        }}
      />
      <output aria-label="Selected values">{value.join(", ") || "None"}</output>
      <output aria-label="Apply count">{applies}</output>
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
          "A reusable cxz multi-select dropdown built from Nox MenuSurface, MenuItem and MenuScrollArea through AnchoredMenu. Changes stay in a local draft while the menu is open. Closing via its trigger, Escape, outside click or focus departure calls choose once, only if the selection changed. Arrow keys/Home/End move through entries and Space/Enter toggle. The All row supports mixed state and preserves disabled selections. Trigger text stays fixed. Placement, glass, corners and entrance animation use the shared menu implementation.",
      },
    },
  },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const DisabledOption: Story = { args: { disabledLast: true } };
