import {
  TerminalWorkbench as Workbench,
  type TerminalWorkbenchProps,
} from "@lesomnus/cxz-ui/terminal";
import { translateKnown } from "#src/shared/i18n/i18n.ts";
import styles from "./workspace-terminal.module.css";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";

// Application adapter: generic docking UI has no cxz connection or locale store.
export function TerminalWorkbench({
  projectError,
  ...props
}: Pick<TerminalWorkbenchProps, "visible" | "hide" | "renderSession"> & {
  projectError?: string;
}) {
  useLocale();
  return (
    <Workbench
      {...props}
      className={`workspace-terminal ${styles.workbench}`}
      error={projectError}
      hideShortcut="Ctrl+`"
      formatStatus={translateKnown}
    />
  );
}
