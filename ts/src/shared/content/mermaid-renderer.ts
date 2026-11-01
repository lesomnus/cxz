import mermaid from "mermaid";

export type MermaidAppearance = {
  dark: boolean;
  background: string;
  surface: string;
  secondary: string;
  foreground: string;
  line: string;
  fontFamily: string;
  fontSize: string;
};
export type MermaidImage = { svg: string; width: number; height: number };

const MAX_SOURCE = 50_000;
const MAX_ENTRIES = 32;
const MAX_BYTES = 2 * 1024 * 1024;
const images = new Map<string, { image: MermaidImage; bytes: number }>();
let retainedBytes = 0;
let nextId = 0;
let queue: Promise<unknown> = Promise.resolve();

export function renderMermaid(
  text: string,
  appearance: MermaidAppearance,
  signal: AbortSignal,
): Promise<MermaidImage | undefined> {
  // initialize changes global state. Serialize it with rendering so concurrent
  // blocks, theme changes and StrictMode cannot use another request's config.
  const request = queue.then(async () => {
    if (signal.aborted) return;
    if (text.length > MAX_SOURCE)
      throw new Error("Mermaid source is too large");
    const key = JSON.stringify([appearance, text]);
    const cached = images.get(key);
    if (cached) {
      images.delete(key);
      images.set(key, cached);
      return cached.image;
    }
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      suppressErrorRendering: true,
      htmlLabels: false,
      maxTextSize: MAX_SOURCE,
      maxEdges: 500,
      theme: "base",
      look: "classic",
      themeCSS: "",
      fontFamily: appearance.fontFamily,
      secure: [
        "secure",
        "securityLevel",
        "startOnLoad",
        "maxTextSize",
        "suppressErrorRendering",
        "maxEdges",
        "htmlLabels",
        "themeCSS",
        "fontFamily",
        "theme",
        "themeVariables",
      ],
      themeVariables: {
        darkMode: appearance.dark,
        background: appearance.background,
        primaryColor: appearance.surface,
        primaryTextColor: appearance.foreground,
        primaryBorderColor: appearance.line,
        secondaryColor: appearance.secondary,
        secondaryTextColor: appearance.foreground,
        secondaryBorderColor: appearance.line,
        tertiaryColor: appearance.surface,
        tertiaryTextColor: appearance.foreground,
        tertiaryBorderColor: appearance.line,
        lineColor: appearance.line,
        textColor: appearance.foreground,
        edgeLabelBackground: appearance.background,
        fontFamily: appearance.fontFamily,
        fontSize: appearance.fontSize,
      },
    });
    const staging = document.createElement("div");
    staging.style.cssText =
      "position:fixed;left:-100000px;top:0;visibility:hidden;pointer-events:none";
    staging.setAttribute("aria-hidden", "true");
    document.body.append(staging);
    try {
      const { svg } = await mermaid.render(
        `cxz-mermaid-${++nextId}`,
        text,
        staging,
      );
      if (signal.aborted) return;
      const doc = new DOMParser().parseFromString(svg, "image/svg+xml");
      const root = doc.documentElement;
      const [, , width, height] = (root.getAttribute("viewBox") ?? "")
        .trim()
        .split(/[\s,]+/)
        .map(Number);
      if (root.localName !== "svg" || !(width > 0 && height > 0))
        throw new Error("Mermaid did not produce an SVG viewport");
      // The SVG is displayed as an image: its styles and generated IDs stay
      // isolated from the application, including duplicate cached diagrams.
      root.setAttribute("width", String(width));
      root.setAttribute("height", String(height));
      const image = {
        svg: new XMLSerializer().serializeToString(root),
        width,
        height,
      };
      const bytes = 2 * (key.length + image.svg.length);
      if (bytes <= MAX_BYTES) {
        while (
          images.size >= MAX_ENTRIES ||
          retainedBytes + bytes > MAX_BYTES
        ) {
          const oldest = images.keys().next().value!;
          retainedBytes -= images.get(oldest)!.bytes;
          images.delete(oldest);
        }
        images.set(key, { image, bytes });
        retainedBytes += bytes;
      }
      return image;
    } finally {
      staging.remove();
    }
  });
  queue = request.catch(() => {});
  return request;
}
