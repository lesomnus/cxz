import { SourceEditor as UIEditor } from "@lesomnus/cxz-ui/editor";
import type { ComponentProps } from "react";
import { useEditorSettings } from "../settings/settings";
import { useTheme } from "../theme/theme";
export type { SourceEditorHandle } from "@lesomnus/cxz-ui/editor";

// The app adapter resolves persisted settings; the UI editor accepts only values.
export function SourceEditor(
  props: Omit<ComponentProps<typeof UIEditor>, "settings" | "theme">,
) {
  const settings = useEditorSettings();
  const theme = useTheme();
  return <UIEditor {...props} settings={settings} theme={theme} />;
}
