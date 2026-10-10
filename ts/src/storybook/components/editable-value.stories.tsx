import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { EditableValue } from "@lesomnus/cxz-ui";
import { validateSessionField } from "../../features/session/model/session-edit";

const meta = {
  title: "Components/EditableValue",
  component: EditableValue,
  tags: ["autodocs"],
  args: {
    label: "Alias",
    value: "oak",
    monospace: true,
    validate: (value) => validateSessionField("alias", value),
    save: async (_value: string) => {},
  },
  parameters: {
    docs: {
      description: {
        component:
          "Click a hoverable value to edit in the same bounds. A native modal dims and disables the surrounding page; Cancel and Confirm float below the input. Enter confirms, Escape or the backdrop cancels. Validation and save errors retain the draft for retry; pending saves block dismissal and duplicate submission. Opening and submitting the editor never submit a containing form.",
      },
    },
  },
  render: function Interactive(args) {
    const [value, setValue] = useState(args.value);
    const [submits, setSubmits] = useState(0);
    return (
      <form
        onSubmit={(event) => {
          event.preventDefault();
          setSubmits((n) => n + 1);
        }}
      >
        <dl className="session-details" style={{ width: "min(420px, 100%)" }}>
          <div>
            <dt>{args.label}</dt>
            <dd>
              <EditableValue
                {...args}
                value={value}
                save={async (next) => {
                  await args.save(next);
                  setValue(next);
                }}
              />
            </dd>
          </div>
        </dl>
        <p role="status">Parent submissions: {submits}</p>
      </form>
    );
  },
} satisfies Meta<typeof EditableValue>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Alias: Story = {};
export const Title: Story = {
  args: {
    label: "Title",
    value: "UI exploration",
    monospace: false,
    validate: (value) => validateSessionField("name", value),
  },
};
export const Pending: Story = {
  args: {
    save: () => new Promise<void>((resolve) => setTimeout(resolve, 1500)),
  },
};
export const Failure: Story = {
  args: {
    save: async (value) => {
      if (value === "taken") throw new Error("Session alias is already in use");
    },
  },
};
