import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { AccountList } from "#src/features/accounts/account-list.tsx";
import { MemoryLibrary } from "#src/features/memory/memory-library.tsx";
import { UsageReport } from "#src/features/activity/usage-report.tsx";
import { BackgroundReport } from "#src/features/activity/background-report.tsx";
import { AuxiliarySettings } from "#src/features/auxiliary/auxiliary-settings.tsx";
import { resourceFixture } from "./resource-fixture";
function ResourcePreview({
  view,
}: {
  view: "accounts" | "memory" | "usage" | "background" | "auxiliary";
}) {
  const [c] = useState(resourceFixture);
  return (
    <div
      style={{
        width: "min(600px, 100vw)",
        padding: "var(--space)",
        height: "95vh",
        overflow: "auto",
      }}
    >
      {view === "accounts" ? (
        <AccountList c={c} />
      ) : view === "memory" ? (
        <MemoryLibrary c={c} session="demo" />
      ) : view === "usage" ? (
        <UsageReport c={c} />
      ) : view === "background" ? (
        <BackgroundReport c={c} session="demo" run="run" />
      ) : (
        <AuxiliarySettings c={c} />
      )}
    </div>
  );
}
const meta = {
  title: "Resources/Views",
  component: ResourcePreview,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The connected resource components with isolated, typed in-memory RPC fixtures. Provider login needs a real Manager.",
      },
    },
  },
  args: { view: "memory" },
} satisfies Meta<typeof ResourcePreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const SharedMemory: Story = {};
export const Accounts: Story = { args: { view: "accounts" } };
export const Usage: Story = { args: { view: "usage" } };
export const BackgroundTasks: Story = { args: { view: "background" } };
export const AuxiliaryAI: Story = { args: { view: "auxiliary" } };
