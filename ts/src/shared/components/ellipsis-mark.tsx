import { Icon } from "#src/shared/icons/icon.tsx";
import styles from "./ellipsis-mark.module.css";

// One text character wide, with the three dots at the line's visual center.
export function EllipsisMark() {
  return <Icon name="overflow" className={styles.mark} aria-hidden="true" />;
}
