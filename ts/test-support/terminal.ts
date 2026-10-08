import type { Locator, Page } from "@playwright/test";

// Existing shell/scroll tests inspect DOM text. Keep those as explicit coverage
// of machines without WebGL; renderer tests exercise the canvas separately.
export async function disableTerminalWebGL(page: Page) {
  await page.addInitScript(() => {
    const getContext = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (
      this: HTMLCanvasElement,
      type,
      ...args
    ) {
      if (type === "webgl2") return null;
      return getContext.call(this, type, ...args);
    } as typeof getContext;
  });
}

export async function loseTerminalContext(panel: Locator) {
  await panel.locator(".xterm-screen canvas").evaluateAll((canvases) => {
    const canvas = canvases.find((el) =>
      (el as HTMLCanvasElement).getContext("webgl2"),
    ) as HTMLCanvasElement;
    const gl = canvas?.getContext("webgl2");
    const extension = gl?.getExtension("WEBGL_lose_context");
    if (!extension)
      throw new Error("Test browser must support WebGL context loss");
    extension.loseContext();
  });
}

export async function copyTerminalSelection(panel: Locator) {
  return panel.locator(".xterm").evaluate((el) => {
    const clipboardData = new DataTransfer();
    el.dispatchEvent(
      new ClipboardEvent("copy", {
        bubbles: true,
        cancelable: true,
        clipboardData,
      }),
    );
    return clipboardData.getData("text/plain");
  });
}
