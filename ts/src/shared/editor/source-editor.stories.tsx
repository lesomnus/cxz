import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { SourceEditor } from "./source-editor";
import "../../storybook/preview.css";

const meta = {
  title: "Editor/SourceEditor",
  component: SourceEditor,
  argTypes: { value: { control: false } },
  tags: ["autodocs"],
  args: {
    path: "example.ts",
    languageId: "typescript",
    ariaLabel: "File preview",
    value:
      "export function greet(name: string) {\n  return `Hello, ${name}`;\n}\n",
    readOnly: true,
  },
  decorators: [
    (Story) => (
      <div className="storybook-source-editor">
        <Story />
      </div>
    ),
  ],
  render: function Editor(args) {
    const [value, setValue] = useState(args.value);
    return <SourceEditor {...args} value={value} onChange={setValue} />;
  },
} satisfies Meta<typeof SourceEditor>;
export default meta;
type Story = StoryObj<typeof meta>;
export const FilePreview: Story = {};
export const SettingsJson: Story = {
  args: {
    path: "settings.json",
    languageId: "json",
    ariaLabel: "Settings file",
    readOnly: false,
    value:
      '{\n  "editor.tabSize": 4,\n  "editor.colorPalette": "muted",\n  "session.editor.indentSize": 2\n}\n',
  },
};
export const Empty: Story = { args: { value: "" } };
