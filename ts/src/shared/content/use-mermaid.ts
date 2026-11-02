import { useEffect, useRef, useState } from "react";
import { useTheme } from "#src/shared/theme/theme.tsx";
import type { MermaidAppearance } from "./mermaid-renderer";

type Preview =
  | { status: "loading" | "error" }
  | { status: "ready"; url: string; width: number; height: number };

function appearance(dark: boolean): MermaidAppearance {
  const tokens = getComputedStyle(document.documentElement);
  const body = getComputedStyle(document.body);
  const shade = (value: number) =>
    tokens.getPropertyValue(`--ui-shade-${value}`).trim();
  return {
    dark,
    background: shade(16),
    surface: shade(41),
    secondary: shade(24),
    foreground: shade(237),
    line: shade(115),
    fontFamily: body.fontFamily,
    fontSize: body.fontSize,
  };
}
function clearImage(image: { current: string | undefined }) {
  if (image.current) URL.revokeObjectURL(image.current);
  image.current = undefined;
}

export function useMermaid(text: string, enabled: boolean): Preview {
  const theme = useTheme();
  const key = enabled ? JSON.stringify([text, theme]) : "";
  const imageUrl = useRef<string | undefined>(undefined);
  const [state, setState] = useState<{
    key: string;
    text: string;
    preview: Preview;
  }>();
  useEffect(() => () => clearImage(imageUrl), []);
  useEffect(() => {
    if (!enabled) {
      clearImage(imageUrl);
      setState(undefined);
      return;
    }
    const controller = new AbortController();
    // Read tokens after the ThemeProvider's layout effect has applied them.
    // Coalesce streaming chunks; canceled work never joins the render queue.
    const timer = setTimeout(() => {
      const colors = appearance(theme === "dark");
      void import("./mermaid-renderer")
        .then(({ renderMermaid }) =>
          renderMermaid(text, colors, controller.signal),
        )
        .then((image) => {
          if (!image || controller.signal.aborted) return;
          const url = URL.createObjectURL(
            new Blob([image.svg], { type: "image/svg+xml" }),
          );
          clearImage(imageUrl);
          imageUrl.current = url;
          setState({
            key,
            text,
            preview: {
              status: "ready",
              url,
              width: image.width,
              height: image.height,
            },
          });
        })
        .catch(() => {
          if (!controller.signal.aborted) {
            clearImage(imageUrl);
            setState({ key, text, preview: { status: "error" } });
          }
        });
    }, 180);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [text, theme, key, enabled]);
  // Keep a loaded image and its geometry until its new theme is ready. Source
  // changes show their own definition immediately, never stale content.
  return enabled &&
    state?.text === text &&
    (state.preview.status === "ready" || state.key === key)
    ? state.preview
    : { status: "loading" };
}
