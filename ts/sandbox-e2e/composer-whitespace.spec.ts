import { expect, test, type Locator } from "@playwright/test";

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
  permissions: ["clipboard-read", "clipboard-write"],
});

async function textGeometry(mirror: Locator) {
  return mirror.evaluate((element) => {
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    let node: Node | null;
    let tail: DOMRect | undefined;
    let space: DOMRect | undefined;
    while ((node = walker.nextNode())) {
      const text = node.textContent!;
      const range = document.createRange();
      if (!space && text.includes(" ")) {
        range.setStart(node, text.indexOf(" "));
        range.setEnd(node, text.indexOf(" ") + 1);
        space = range.getBoundingClientRect();
      }
      const index = text.lastIndexOf("abcd");
      if (index < 0) continue;
      range.setStart(node, index);
      range.setEnd(node, index + 4);
      tail = range.getBoundingClientRect();
    }
    const origin = element.getBoundingClientRect();
    return {
      x: tail!.x - origin.x,
      y: tail!.y - origin.y,
      space: space!.width,
    };
  });
}

for (const decorated of [false, true]) {
  test(`consecutive spaces keep their width across soft wraps (decorated=${decorated})`, async ({
    page,
  }) => {
    await page.goto("/sandbox.html#/sessions/session-1");
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    await expect(input).toBeEditable({ timeout: 45000 });
    const editor = input.locator("..").locator("..");
    const mirror = editor.locator(".editor-mirror");
    const prefix =
      (decorated ? "`abcd` " : "abcd ") + "abcd ".repeat(12) + "abcd";
    const positions = [];
    for (const controls of [false, true]) {
      if (controls) {
        await page
          .getByRole("button", { name: "Session menu", exact: true })
          .click();
        await page
          .getByRole("button", { name: "Show control characters", exact: true })
          .click();
        await page.keyboard.press("Escape");
      }
      const geometries = [];
      for (const spaces of [2, 5]) {
        const value = prefix + " ".repeat(spaces) + "abcd ";
        await input.fill(value);
        await expect(editor).toHaveAttribute(
          "data-decorated",
          String(decorated),
        );
        await expect(input).toHaveValue(value);
        await expect(input).toHaveCSS("white-space", "break-spaces");
        const geometry = await textGeometry(mirror);
        geometries.push(geometry);
        if (controls) {
          const markers = editor.locator(".editor-control-mirror");
          const marked = await textGeometry(markers);
          expect(Math.abs(marked.x - geometry.x)).toBeLessThanOrEqual(1 / 8);
          expect(marked.y).toBeCloseTo(geometry.y, 1);
        }
        // Rendering preserves source spaces, including clipboard and selection.
        await input.press("Control+a");
        await input.press("Control+c");
        expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
          value,
        );
      }
      expect(geometries[1].y).toBe(geometries[0].y);
      // Independently laid-out text runs can differ by a few layout subpixels.
      expect(
        Math.abs(geometries[1].x - geometries[0].x - 3 * geometries[0].space),
      ).toBeLessThanOrEqual(1 / 8);
      positions.push(geometries);
    }
    expect(positions[1]).toEqual(positions[0]);
  });
}
