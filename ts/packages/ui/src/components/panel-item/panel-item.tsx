import {
  forwardRef,
  type AnchorHTMLAttributes,
  type ComponentProps,
} from "react";
import { Button, ButtonContent } from "../button/button";
import styles from "./panel-item.module.css";

// Selection follows the native accessibility state for both buttons and links.
export function PanelItem({
  className = "",
  ...props
}: ComponentProps<typeof Button>) {
  return <Button {...props} className={`${styles.item} ${className}`} />;
}

export const PanelItemAnchor = forwardRef<
  HTMLAnchorElement,
  AnchorHTMLAttributes<HTMLAnchorElement> & { pressTarget?: string }
>(function PanelItemAnchor(
  { children, pressTarget, className = "", ...props },
  ref,
) {
  return (
    <a
      {...props}
      ref={ref}
      className={`${styles.item} route-link ${className}`}
    >
      <ButtonContent pressTarget={pressTarget}>{children}</ButtonContent>
    </a>
  );
});
