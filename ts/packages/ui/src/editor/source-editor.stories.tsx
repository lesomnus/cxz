import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SourceEditor } from "@lesomnus/cxz-ui/editor";
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
      <div style={{ height: 420 }}>
        <Story />
      </div>
    ),
  ],
  render: function Interactive(args, context) {
    const [value, setValue] = useState(args.value);
    return (
      <SourceEditor
        {...args}
        theme={context.globals.theme === "light" ? "light" : "dark"}
        value={value}
        onChange={setValue}
      />
    );
  },
} satisfies Meta<typeof SourceEditor>;
export default meta;
type Story = StoryObj<typeof meta>;
export const FilePreview: Story = {};
export const Editable: Story = {
  args: {
    readOnly: false,
    settings: { tabSize: 2, insertSpaces: true, colorPalette: "cool" },
  },
};

export const SettingsJson: Story = {
  args: {
    path: "settings.json",
    languageId: "json",
    readOnly: false,
    value: '{"editor.tabSize": 4}',
  },
};
