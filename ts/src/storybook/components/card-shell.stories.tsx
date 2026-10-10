import type { Meta, StoryObj } from "@storybook/react-vite";
import { FloatingCard } from "@lesomnus/cxz-ui";
import { DetailTabs } from "../../features/session/conversation/detail-tabs";
import "../preview.css";

const meta = {
  title: "Components/FloatingCard",
  component: FloatingCard,
  tags: ["autodocs"],
  args: {
    title: "Paste preview",
    closeLabel: "Close paste preview",
    close: () => {},
    children: <pre>{"export const ready = true;\nconsole.log(ready);"}</pre>,
  },
  render: (args) => (
    <FloatingCard {...args} data-entered="true" data-active="true" />
  ),
  decorators: [
    (Story) => (
      <div className="conversation storybook-card">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof FloatingCard>;
export default meta;
type Story = StoryObj<typeof meta>;
export const PastePreview: Story = {};
export const EditorTabs: Story = {
  args: {
    title: "Task details",
    hideHeading: true,
    children: (
      <DetailTabs
        sections={[
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
        ]}
      />
    ),
  },
};
