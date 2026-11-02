import { useEffect, useState, type CSSProperties } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SectionGroup } from "@lesomnus/nox";
import { create } from "@bufbuild/protobuf";
import { SessionSchema } from "#gen/cxz/session_pb";
import { AccountQuotaSquare } from "#src/features/accounts/account-quota-square.tsx";
import { resourceFixture } from "../resource-fixture";

const sessions = [
  create(SessionSchema, { runtimeId: "demo-review", agent: "codex" }),
];

function QuotaFixture({ remaining }: { remaining: number | null }) {
  const [c] = useState(() => resourceFixture({ quotaRemaining: remaining }));
  useEffect(() => () => c.dispose(), [c]);
  return (
    <SectionGroup
      style={
        {
          "--ui-section-radius": "var(--ui-section-inner-radius)",
        } as CSSProperties
      }
    >
      <AccountQuotaSquare
        c={c}
        account="work"
        agent="codex"
        sessions={sessions}
      />
    </SectionGroup>
  );
}

function QuotaPreview({ remaining = 64 }: { remaining: number | null }) {
  return <QuotaFixture key={remaining ?? "unknown"} remaining={remaining} />;
}

const meta = {
  title: "Accounts/Quota",
  component: QuotaPreview,
  args: { remaining: 64 },
  argTypes: {
    remaining: {
      control: { type: "number", min: 0, max: 100, step: 1 },
      description:
        "Provider-reported percentage remaining; null means not reported.",
    },
  },
  parameters: {
    docs: {
      description: {
        component:
          "The Accounts page's actual 64×64 quota display, backed by an isolated in-memory RPC fixture. The container has a 16px radius; the 32×60 gauge has a 2px inset and a 14px radius so their left corner centers align. The gauge stays tinted across its full height. An empty-area box with the same 14px radius grows downward from the top as quota is used, inset 1px inside the gauge; its maximum height leaves a 1px bottom gap too. The percentage stays above both layers and uses CompactNumber. The reset countdown stacks days, hours and minutes to its right. Critical usage is below 10%, warning is 10–30%, and normal is above 30%. Use the theme toolbar to compare dark and light appearances. No Manager, provider or saved account is used.",
      },
    },
  },
} satisfies Meta<typeof QuotaPreview>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Comparison: Story = {
  render: () => (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(auto-fit, minmax(80px, 1fr))",
        gap: "24px 16px",
        width: "min(720px, calc(100vw - 48px))",
        paddingBlock: 24,
      }}
    >
      {[0, 1, 8, 10, 30, 31, 64, 99, 100, null].map((remaining) => (
        <figure
          key={remaining ?? "unknown"}
          aria-label={
            remaining === null ? "Not reported" : `${remaining}% remaining`
          }
          style={{ margin: 0, display: "grid", justifyItems: "center", gap: 8 }}
        >
          <figcaption style={{ color: "var(--muted)", fontSize: 12 }}>
            {remaining === null ? "Not reported" : `${remaining}%`}
          </figcaption>
          <QuotaPreview remaining={remaining} />
        </figure>
      ))}
    </div>
  ),
  parameters: { controls: { disable: true } },
};

export const Interactive: Story = {};
export const NotReported: Story = { args: { remaining: null } };
