import type { Meta, StoryObj } from "@storybook/react-vite";
import { FloatingCard } from "@lesomnus/nox";
import { DetailTabs } from "#src/features/session/conversation/detail-tabs.tsx";
import "#src/storybook/preview.css";

const meta = {
  title: "Conversation/TaskDetails",
  component: DetailTabs,
  tags: ["autodocs"],
  args: {
    sections: [
      {
        id: "input",
        label: "Input",
        language: "json",
        value: '{\n  "command": "git status --short"\n}',
      },
      {
        id: "output",
        label: "Output",
        value: "## main\n M ts/src/app.tsx\n",
      },
      {
        id: "result",
        label: "Result",
        language: "json",
        value: '{ "exitCode": 0 }',
      },
    ],
  },
  render: (args) => (
    <FloatingCard
      title="Task details"
      closeLabel="Close task details"
      close={() => {}}
      hideHeading
      data-entered="true"
      data-active="true"
    >
      <DetailTabs {...args} />
    </FloatingCard>
  ),
  decorators: [
    (Story) => (
      <div className="conversation storybook-card">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof DetailTabs>;
export default meta;
type Story = StoryObj<typeof meta>;
export const EditorTabs: Story = {};
