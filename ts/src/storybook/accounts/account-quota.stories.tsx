import { useEffect, useRef, useState, type CSSProperties } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SectionGroup, Slider } from "@lesomnus/nox";
import { create } from "@bufbuild/protobuf";
import { SessionSchema } from "#gen/cxz/session_pb";
import { AccountQuotaSquare } from "#src/features/accounts/account-quota-square.tsx";
import { resourceFixture } from "../resource-fixture";

const sessions = [
  create(SessionSchema, { runtimeId: "demo-review", agent: "codex" }),
];

function QuotaFixture({ remaining }: { remaining: number | null }) {
  const current = useRef(remaining);
  current.current = remaining;
  const [c] = useState(() =>
    resourceFixture({ quotaRemaining: () => current.current }),
  );
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
        refreshVersion={remaining ?? -1}
      />
    </SectionGroup>
  );
}

function QuotaPreview({ remaining = 64 }: { remaining: number | null }) {
  return <QuotaFixture remaining={remaining} />;
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
          "The Accounts page's actual remaining-usage display, backed by an isolated in-memory RPC fixture. The original rounded narrow tube and single liquid tint are retained. The liquid has a concave meniscus: higher against the walls and lower at the center. A fixed-size empty cap moves down as usage is consumed, so its curve never compresses near full capacity. The tube clips the cap at its ends; dimensions and motion live in CSS tokens. The percentage remains centered above the gauge and the reset countdown stays beside it. Existing warning and critical colors follow the remaining quota. Water Level lets you drain and refill the same mounted gauge; use the theme toolbar to compare appearances.",
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
      {[0, 1, 8, 10, 30, 31, 64, 90, 95, 98, 99, 100, null].map((remaining) => (
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
export const WaterLevel: Story = {
  render: function WaterLevel() {
    const [remaining, setRemaining] = useState(95);
    return (
      <div
        style={{
          display: "grid",
          gap: "var(--ui-section-layout-gap)",
          width: "min(280px, calc(100vw - 48px))",
        }}
      >
        <QuotaPreview remaining={remaining} />
        <Slider
          label="Remaining usage"
          value={remaining}
          onChange={setRemaining}
          min={0}
          max={100}
        />
      </div>
    );
  },
  parameters: { controls: { disable: true } },
};
export const NotReported: Story = { args: { remaining: null } };
