import type { Meta, StoryObj } from "@storybook/react-vite";
import { Icon, iconDefinitions, projectIconNames, type IconName } from "./icon";
import styles from "./icon.stories.module.css";

const names = Object.keys(iconDefinitions) as IconName[];
const meta = {
  title: "Foundations/Icons",
  component: Icon,
  args: { name: "start" },
  argTypes: { name: { control: "select", options: names } },
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Reference icons by semantic name. Geometry lives in shared/icons/icon-definitions.ts; aliases share the same definition. Icons inherit currentColor and the shared --ui-icon-stroke-width override. Basic shape glyphs are solid. Unanswered questions, unchecked controls and partial progress retain their state-specific outlines. Dynamic charts and loading animations remain in their own components.",
      },
    },
  },
} satisfies Meta<typeof Icon>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Catalogue: Story = {
  render: () => (
    <ul className={styles.catalogue}>
      {names.map((name) => (
        <li key={name}>
          <Icon name={name} className={styles.glyph} />
          <code>{name}</code>
        </li>
      ))}
    </ul>
  ),
};

// The palette a person picks a project's icon from. These are not meanings: a
// leaf says nothing about a project, it only tells it apart from the one with
// the rocket. They are reviewed here as a set, because that is how they are
// used -- side by side in a list, at the size of the label beside them.
export const ProjectPalette: Story = {
  render: () => (
    <ul className={styles.catalogue}>
      {projectIconNames.map((name) => (
        <li key={name}>
          <Icon name={name} className={styles.glyph} />
          <code>{name}</code>
        </li>
      ))}
    </ul>
  ),
};
