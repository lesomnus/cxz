import { Button, type ButtonProps } from "@lesomnus/nox";
import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import styles from "./toolbar-icon-button.module.css";

export type ToolbarIconButtonProps = Omit<
  ButtonProps,
  "children" | "variant" | "aria-label" | "title"
> & {
  icon: IconName;
  label: string;
};

export function ToolbarIconButton({
  icon,
  label,
  type = "button",
  ...props
}: ToolbarIconButtonProps) {
  return (
    <Button
      {...props}
      type={type}
      variant="toolbar"
      aria-label={label}
      title={label}
    >
      <Icon name={icon} className={styles.icon} />
    </Button>
  );
}
