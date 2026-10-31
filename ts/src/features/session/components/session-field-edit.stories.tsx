import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { EditableValue } from "@lesomnus/nox";
import { validateSessionField } from "#src/features/session/model/session-edit.ts";

const meta = {
  title: "Sessions/FieldEditing",
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
          "Session alias and title editing using Nox's EditableValue and cxz's validateSessionField. Alias validation requires 3–20 lowercase letters, digits and single hyphens, starting with a letter; titles must not be empty. Saves are simulated here. Generic editor appearance and behavior are documented in Nox's Components/EditableValue catalog.",
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
export const Alias: Story = {
  parameters: {
    docs: {
      description: {
        story:
          "Edit a session alias with cxz's alias validation and monospace value styling.",
      },
    },
  },
};
export const Title: Story = {
  args: {
    label: "Title",
    value: "UI exploration",
    monospace: false,
    validate: (value) => validateSessionField("name", value),
  },
};
export const Pending: Story = {
  name: "Saving alias",
  args: {
    save: () => new Promise<void>((resolve) => setTimeout(resolve, 1500)),
  },
};
export const Failure: Story = {
  name: "Alias conflict",
  parameters: {
    docs: {
      description: {
        story:
          'Enter "taken" to simulate a conflicting session alias, then choose another alias to retry.',
      },
    },
  },
  args: {
    save: async (value) => {
      if (value === "taken") throw new Error("Session alias is already in use");
    },
  },
};
