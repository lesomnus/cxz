import type { Meta, StoryObj } from "@storybook/react-vite";
import { UsageInfo } from "./usage-info";

const meta = {
  title: "Conversation/Usage",
  component: UsageInfo,
  decorators: [
    (Story) => (
      <div
        style={{
          padding: "180px 24px 24px",
          width: 520,
          maxWidth: "100vw",
        }}
      >
        <Story />
      </div>
    ),
  ],
  args: {
    agent: "codex",
    info: {
      model: "",
      effort: "",
      remaining: 100,
      quotaLabel: "codex · 5h",
      reset: Date.now() + 5 * 3600_000,
      contextUsed: 48000,
      contextWindow: 200000,
    },
  },
} satisfies Meta<typeof UsageInfo>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Full: Story = {};
export const Low: Story = {
  args: {
    agent: "claude",
    info: {
      ...meta.args.info,
      remaining: 1,
      quotaLabel: "five hour",
      reset: Date.now() + 60000,
    },
  },
};
export const Weekly: Story = {
  args: {
    info: {
      ...meta.args.info,
      remaining: 72,
      quotaLabel: "codex · weekly",
      reset: Date.now() + 7 * 24 * 3600_000,
    },
  },
};
export const SessionAndWeekly: Story = {
  args: {
    info: {
      ...meta.args.info,
      remaining: 8,
      quotaLabel: "codex · weekly",
      reset: Date.now() + 6.5 * 24 * 3600_000,
      quotas: [
        {
          key: "codex/primary",
          label: "codex · 5h",
          bucket: "codex",
          period: "5h",
          remaining: 72,
          reset: Date.now() + 5 * 3600_000,
        },
        {
          key: "codex/secondary",
          label: "codex · weekly",
          bucket: "codex",
          period: "weekly",
          remaining: 8,
          reset: Date.now() + 6.5 * 24 * 3600_000,
        },
      ],
    },
  },
};
export const MultipleBuckets: Story = {
  args: {
    info: {
      ...SessionAndWeekly.args!.info!,
      quotas: [
        ...SessionAndWeekly.args!.info!.quotas!,
        {
          key: "spark/primary",
          label: "Spark · 5h",
          bucket: "spark",
          bucketName: "Spark",
          period: "5h",
          remaining: 24,
          reset: Date.now() + 4 * 3600_000,
        },
        {
          key: "spark/secondary",
          label: "Spark · weekly",
          bucket: "spark",
          bucketName: "Spark",
          period: "weekly",
          remaining: 90,
          reset: Date.now() + 4 * 24 * 3600_000,
        },
      ],
    },
  },
};
export const ClaudeWindows: Story = {
  args: {
    agent: "claude",
    info: {
      ...meta.args.info,
      remaining: 15,
      quotaLabel: "seven day opus",
      quotas: [
        {
          key: "five_hour",
          label: "five hour",
          period: "five_hour",
          remaining: 70,
          reset: Date.now() + 5 * 3600_000,
        },
        {
          key: "seven_day",
          label: "seven day",
          period: "seven_day",
          remaining: 60,
          reset: Date.now() + 6 * 24 * 3600_000,
        },
        {
          key: "seven_day_opus",
          label: "seven day opus",
          period: "seven_day_opus",
          remaining: 15,
          reset: Date.now() + 6 * 24 * 3600_000,
        },
        {
          key: "seven_day_sonnet",
          label: "seven day sonnet",
          period: "seven_day_sonnet",
          status: "allowed",
        },
        {
          key: "model_scoped/0",
          label: "Fable",
          remaining: 35,
          reset: Date.now() + 2 * 24 * 3600_000,
        },
      ],
    },
  },
};
export const Unreported: Story = { args: { info: { model: "", effort: "" } } };

export const SmallContext: Story = {
  args: { info: { ...meta.args.info, contextUsed: 123, contextWindow: 999 } },
};

export const Warning: Story = {
  args: { info: { ...meta.args.info, remaining: 30 } },
};
export const WarningBoundary: Story = {
  args: { info: { ...meta.args.info, remaining: 10 } },
};
export const Exhausted: Story = {
  args: { info: { ...meta.args.info, remaining: 0 } },
};

export const InsideComposer: Story = {
  render: (args) => (
    <div className="composer" style={{ width: "100%" }}>
      <div className="composer-input" style={{ height: 120 }} />
      <div className="composer-meta">
        <UsageInfo {...args} />
      </div>
    </div>
  ),
};
