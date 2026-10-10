import type { Meta, StoryObj } from "@storybook/react-vite";
import { useArgs } from "storybook/preview-api";
import { FontFamilyControl } from "./font-family-control";
import { defaultEditorSettings } from "./editor-settings";

const meta = {
  title: "Components/FontFamilyControl",
  component: FontFamilyControl,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Local CSS font lists and Google Fonts share one control. Google Fonts makes no requests until Apply font; the editor switches after the download is ready. Saved selections contain only provider/family metadata. Removing an override restores the complete inherited selection.",
      },
    },
  },
  args: {
    label: "Editor Font family",
    inherited: defaultEditorSettings.fontFamily,
    onChange: () => {},
  },
  render: function Interactive(args) {
    const [, updateArgs] = useArgs();
    return (
      <div style={{ maxWidth: 360 }}>
        <FontFamilyControl
          {...args}
          onChange={(value) => updateArgs({ value })}
        />
        <pre>
          {JSON.stringify({ "editor.fontFamily": args.value }, null, 2)}
        </pre>
      </div>
    );
  },
} satisfies Meta<typeof FontFamilyControl>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const LocalCustom: Story = {
  args: { value: '"Liberation Mono", monospace' },
};
