import type { ComponentProps } from "react";
import { ValueMenu } from "@lesomnus/cxz-ui";
import styles from "./resource-surface.module.css";
export function ResourceValue(props: ComponentProps<typeof ValueMenu>) {
  return (
    <div className={styles.field}>
      <span>{props.label}</span>
      <ValueMenu {...props} />
    </div>
  );
}
