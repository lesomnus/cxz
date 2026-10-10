import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
  ActionMenu,
  Button,
  EditableValue,
  FloatingCard,
  SettingField,
  SegmentedControl,
  UIProvider,
} from "@lesomnus/cxz-ui";

const meta = {
  title: "Integration/Standalone",
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "This Storybook imports only the UI package and documentation framing. There are no cxz app styles, API clients, settings stores or language packs.",
      },
    },
  },
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;
export const Editable: Story = {
  render: function Example() {
    const [value, setValue] = useState("My workspace");
    return (
      <EditableValue
        label="Name"
        value={value}
        save={async (next) => {
          setValue(next);
        }}
        validate={(next) => (next ? undefined : "Enter a name.")}
      />
    );
  },
};
export const Card: Story = {
  render: () => (
    <FloatingCard
      title="Preview"
      closeLabel="Close preview"
      close={() => {}}
      footer={<Button>Continue</Button>}
    >
      <pre>{"export const ready = true;\nconsole.log(ready);"}</pre>
    </FloatingCard>
  ),
};
export const Settings: Story = {
  render: function Example() {
    const [value, setValue] = useState("dark");
    return (
      <SettingField
        title="Appearance"
        settingId="ui.theme"
        summary="Choose an appearance for your project."
        details="The application owns the setting and saves it however it chooses."
      >
        <SegmentedControl
          label="Appearance"
          value={value}
          options={[
            { value: "light", label: "Light" },
            { value: "dark", label: "Dark" },
            { value: "system", label: "System" },
          ]}
          onChange={setValue}
        />
      </SettingField>
    );
  },
};
export const Menu: Story = {
  render: function Example() {
    const [last, setLast] = useState("No action");
    return (
      <>
        <ActionMenu
          label="Actions"
          items={[
            { label: "Open", run: () => setLast("Opened") },
            { label: "Archive", run: () => setLast("Archived") },
          ]}
        />
        <p role="status">{last}</p>
      </>
    );
  },
};
export const Translated: Story = {
  render: function Example() {
    const [value, setValue] = useState("Workspace");
    return (
      <UIProvider
        translate={(message, values) =>
          message === "Cancel"
            ? "Dismiss"
            : message.replace(/\{(\w+)\}/g, (match, key) =>
                String(values?.[key] ?? match),
              )
        }
      >
        <EditableValue
          label="Name"
          value={value}
          save={async (next) => {
            setValue(next);
          }}
        />
      </UIProvider>
    );
  },
};
