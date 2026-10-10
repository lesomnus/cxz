import type { Meta, StoryObj } from "@storybook/react-vite";
import { useRef } from "react";
import { FloatingCard } from "@lesomnus/cxz-ui";
import { SessionDetails } from "./session-details";
import { storySession } from "../../../storybook/fixtures";

const session = storySession();
const meta = {
  title: "Components/SessionDetails",
  component: SessionDetails,
  tags: ["autodocs"],
  args: {
    session,
    info: {
      model: session.model,
      effort: "medium",
      remaining: 72,
      contextUsed: 48000,
      contextWindow: 200000,
    },
    edit: async (field, value) => ({ ...session, [field]: value }),
  },
  parameters: {
    docs: {
      description: {
        component:
          "Title edits Session.name, which falls back to alias/runtime ID when absent. Alias is a separate identity field. Keys and values use monospace text with inset values. Hovering an editable row shows a dark row surface and the existing brighter rounded value surface. Click either value to edit in place with a dimmed background and overlaid Cancel/Confirm controls. Production saves titles through AuxRun(TITLE, text), which records a manual title without running a model, and aliases through Patch, then updates the shared store. Runtime and run identities stay unchanged.",
      },
    },
  },
  render: function Interactive(args) {
    const saved = useRef(args.session);
    return (
      <FloatingCard
        title="Session details"
        data-entered="true"
        style={{
          position: "relative",
          inset: "auto",
          width: "min(560px, 100%)",
          maxHeight: "none",
        }}
      >
        <SessionDetails
          {...args}
          edit={async (field, value) => {
            const result = await args.edit(field, value);
            saved.current = { ...saved.current, [field]: result[field] };
            return saved.current;
          }}
        />
      </FloatingCard>
    );
  },
} satisfies Meta<typeof SessionDetails>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
