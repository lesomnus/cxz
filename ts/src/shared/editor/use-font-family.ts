import { useEffect, useRef, useSyncExternalStore } from "react";
import { defaultEditorSettings } from "../settings/editor-settings";
import {
  googleFontStatus,
  loadGoogleFont,
  subscribeGoogleFonts,
} from "./google-fonts";

export function useFontFamily(fontFamily: string, googleFont?: string) {
  const previous = useRef(defaultEditorSettings.fontFamily);
  const status = useSyncExternalStore(subscribeGoogleFonts, () =>
    googleFontStatus(googleFont),
  );
  useEffect(() => {
    if (googleFont && googleFontStatus(googleFont) === "idle")
      void loadGoogleFont(googleFont).catch(() => {});
  }, [googleFont]);
  useEffect(() => {
    if (!googleFont || status === "ready") previous.current = fontFamily;
  }, [fontFamily, googleFont, status]);
  return {
    fontFamily:
      !googleFont || status === "ready" ? fontFamily : previous.current,
    loading: !!googleFont && (status === "idle" || status === "loading"),
    error: googleFont && status === "error",
    retry: () => {
      if (googleFont) void loadGoogleFont(googleFont).catch(() => {});
    },
  };
}
