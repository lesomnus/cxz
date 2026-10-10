import { SourceEditor as UIEditor } from "@lesomnus/cxz-ui/editor";
import type { ComponentProps } from "react";
import { useEditorSettings } from "#src/shared/settings/settings.ts";
import { useTheme } from "#src/shared/theme/theme.tsx";
export type { SourceEditorHandle } from "@lesomnus/cxz-ui/editor";

// The app adapter resolves persisted settings; the UI editor accepts only values.
export function SourceEditor(
  props: Omit<ComponentProps<typeof UIEditor>, "settings" | "theme">,
) {
  const settings = useEditorSettings();
  const theme = useTheme();
  return <UIEditor {...props} settings={settings} theme={theme} />;
}
