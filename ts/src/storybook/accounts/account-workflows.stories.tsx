import type { Meta, StoryObj } from "@storybook/react-vite";
import { AccountWorkflows } from "./account-workflows";

const meta = {
  title: "Accounts/Workflows",
  component: AccountWorkflows,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Interactive account workflows using the real Accounts page, Add account/Add session forms, LoginPanel and Conversation. Account cards use a bounded two-column layout: the left column groups metadata, conversation usage and authentication at the bottom; the right column uses the shared session table in its compact presentation, with selection and controls. Names and smaller aliases are stacked in one navigation button opening the conversation. The Project column links to the owning project dashboard. Simple state indicators replace state text; agent logos, inline editing and the redundant account column are omitted. Row menus contain lifecycle controls only. Metadata and usage values each use paired fields. Very narrow panes stack the groups. Every story owns an isolated in-memory RPC fixture and a scoped login socket; no Manager, provider connection or real credentials are used. Enter demo-code to finish a login. Reset preview clears accounts, authentication, drafts and pending requests in this example.",
      },
    },
  },
  args: { scenario: "add", loginOutcome: "success" },
  argTypes: {
    scenario: {
      control: "select",
      options: [
        "add",
        "login",
        "signed-in",
        "create-session",
        "send",
        "expired",
      ],
    },
    loginOutcome: {
      control: "select",
      options: ["success", "fail-once", "disconnect-once"],
    },
  },
} satisfies Meta<typeof AccountWorkflows>;
export default meta;
type Story = StoryObj<typeof meta>;

export const AddAccount: Story = {
  parameters: {
    docs: {
      description: {
        story:
          "Start with an empty account list. Use Add account, select Codex or Claude, enter an alias and display name, then Add. Codex accounts support a shared account login; Claude accounts explain that login belongs to each session. Duplicate aliases show the actual form's error state.",
      },
    },
  },
};
export const AccountLogin: Story = {
  args: { scenario: "login" },
  parameters: {
    docs: {
      description: {
        story:
          "The account initially requires login. Click Log in to open the shared modal, submit demo-code and watch the actual account status switch to Signed in. The modal reserves its height before instructions arrive. ScrollArea contains only instructions and errors; the shared Form/FormField code input stays outside the viewport. The header, code field and footer stay in place as content loads or scrolls. Cancel, Escape or clicking the backdrop closes the modal and leaves it signed out. The page layout stays in place. Log out opens the common confirmation dialog and removes the simulated saved login.",
      },
    },
  },
};
export const LoginFailureAndRetry: Story = {
  args: { scenario: "login", loginOutcome: "fail-once" },
  parameters: {
    docs: {
      description: {
        story:
          "The first login attempt returns a provider error. Retry login starts a fresh attempt; demo-code completes it. Entering any other code also shows the retryable error path. Cancel never marks the account signed in.",
      },
    },
  },
};
export const LoginConnectionClosed: Story = {
  args: { scenario: "login", loginOutcome: "disconnect-once" },
  parameters: {
    docs: {
      description: {
        story:
          "The first login connection closes before completion. The panel shows the real disconnect error and Retry login starts a working connection.",
      },
    },
  },
};
export const SignedInAccount: Story = {
  args: { scenario: "signed-in" },
  parameters: {
    docs: {
      description: {
        story:
          "Inspect a signed-in account with usage and an associated session. The compact authentication row contains status on the left and a fixed-width action on the right that switches between Log in and Log out. Log out requires confirmation. Cancel keeps the saved login; confirming switches the action back to Log in. Session-scoped accounts display only an explanation.",
      },
    },
  },
};
export const SessionCreationRequiresLogin: Story = {
  args: { scenario: "create-session" },
  parameters: {
    docs: {
      description: {
        story:
          "Click Add session, choose the Claude work account and Create session. The fixture returns the real PROJECT_LOGIN_REQUIRED ErrorInfo contract. Login opens inside the creation form. Submitting demo-code retries creation with the same creation key and opens the new conversation. Cancel does not create a session.",
      },
    },
  },
};
export const ConversationRequiresLogin: Story = {
  args: { scenario: "send" },
  parameters: {
    docs: {
      description: {
        story:
          "Send the prefilled message. The simulated provider accepts it but fails the turn because OAuth expired, just as a live Claude session can. Use Log in again, submit demo-code and the session resumes. The accepted prompt is not replayed automatically: send another message to receive the local successful response.",
      },
    },
  },
};
export const ExpiredSession: Story = {
  args: { scenario: "expired" },
  parameters: {
    docs: {
      description: {
        story:
          "A conversation initially contains an authentication failure and recovery notice. Log in again opens the usual card above the composer; completing login resumes the session. Cancel or a failed login keeps the failed session intact. Use the loginOutcome control to inspect recovery failures.",
      },
    },
  },
};
