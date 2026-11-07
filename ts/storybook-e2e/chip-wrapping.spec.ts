import { expect, test } from "@playwright/test";
import { caretPosition } from "../src/features/session/composer/editor-caret";

for (const { zoom, longFile } of [
  { zoom: 1, longFile: false },
  { zoom: 1.25, longFile: false },
  { zoom: 1.25, longFile: true },
]) {
  test(`a ${longFile ? "long file" : "text"} chip moves intact to the next soft-wrapped line at zoom ${zoom}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 900 });
    await page.goto(
      "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
    );
    await page.evaluate((zoom) => {
      document.documentElement.style.zoom = String(zoom);
    }, zoom);
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    await input.evaluate((node, longFile) => {
      node.focus();
      const data = new DataTransfer();
      if (longFile) {
        data.items.add(
          new File(["file contents"], `long-${"x".repeat(150)}.txt`),
        );
        node.dispatchEvent(
          new DragEvent("drop", {
            dataTransfer: data,
            bubbles: true,
            cancelable: true,
          }),
        );
      } else {
        data.setData(
          "text/plain",
          "A pasted message long enough for a text chip.",
        );
        node.dispatchEvent(
          new ClipboardEvent("paste", {
            clipboardData: data,
            bubbles: true,
            cancelable: true,
          }),
        );
      }
    }, longFile);
    const chip = page.locator(".paste-chip");
    if (longFile)
      await expect(chip).toHaveAttribute("data-upload-state", "ready");
    else await expect(chip).toHaveAttribute("data-send-as", "text");
    const count = await chip.evaluate((node) => {
      const input = node
        .closest(".composer-editor")!
        .querySelector("textarea")!;
      const mirror = node.closest(".editor-mirror")!;
      const probe = document.createElement("span");
      probe.textContent = "가";
      mirror.append(probe);
      const style = getComputedStyle(input);
      const available =
        parseFloat(style.width) -
        parseFloat(style.paddingLeft) -
        parseFloat(style.paddingRight) -
        (input.offsetWidth - input.clientWidth);
      const count = Math.floor(
        (available - parseFloat(getComputedStyle(node).width) / 2) /
          (probe.getBoundingClientRect().width /
            parseFloat(getComputedStyle(document.documentElement).zoom)),
      );
      probe.remove();
      return count;
    });
    await input.press("Control+Home");
    await input.pressSequentially("가".repeat(count));
    await input.press("Control+End");
    const value = await input.inputValue();
    // Read the native soft-wrap boundary independently of chip selection snapping.
    await input.evaluate((node: HTMLTextAreaElement) => {
      const probe = node.cloneNode() as HTMLTextAreaElement;
      probe.setAttribute("aria-label", "Native wrap probe");
      probe.value = node.value;
      Object.assign(probe.style, {
        position: "absolute",
        top: "0",
        left: "0",
        opacity: "0",
        pointerEvents: "none",
      });
      node.parentElement!.append(probe);
    });
    const native = page.getByRole("textbox", {
      name: "Native wrap probe",
      exact: true,
    });
    await native.press("Control+End");
    await native.press("Home");
    const nativeStart = await native.evaluate(
      (node: HTMLTextAreaElement) => node.selectionStart,
    );
    expect(nativeStart).toBeGreaterThanOrEqual(count);
    expect(nativeStart).toBeLessThanOrEqual(count + 1);
    await native.evaluate((node) => node.remove());
    await input.press("Control+End");
    const bounds = await chip.evaluate((node) => {
      const native = node
        .closest("[data-chip-reservation]")!
        .querySelector("[data-native-chip-text]")!;
      const range = document.createRange();
      range.selectNodeContents(native);
      return {
        chip: node.getBoundingClientRect().toJSON(),
        reservation: range.getBoundingClientRect().toJSON(),
        lines: range.getClientRects().length,
        line: node.closest(".editor-line")!.getBoundingClientRect().toJSON(),
      };
    });
    expect(bounds.lines).toBe(1);
    expect(bounds.chip.y).toBeGreaterThan(
      bounds.line.y + bounds.chip.height / 2,
    );
    expect(bounds.chip.x).toBeCloseTo(bounds.line.x, 0);
    expect(bounds.reservation.right).toBeGreaterThan(bounds.chip.right);
    const end = await input.evaluate(
      (node: HTMLTextAreaElement) => node.selectionEnd,
    );
    const point = await input.evaluate(caretPosition, end);
    const inputBounds = (await input.boundingBox())!;
    expect(inputBounds.y + point.top * zoom).toBeGreaterThan(
      bounds.line.y + bounds.chip.height / 2,
    );
    expect(inputBounds.x + point.left * zoom).toBeGreaterThanOrEqual(
      bounds.chip.right,
    );
    await input.press("Home");
    const start = await input.evaluate(
      (node: HTMLTextAreaElement) => node.selectionStart,
    );
    expect(start).toBeGreaterThanOrEqual(count);
    expect(start).toBeLessThanOrEqual(count + 1);
    await input.press("Alt+z");
    await expect(input).toHaveAttribute("wrap", "off");
    await expect(input).toHaveValue(value);
    const unwrapped = (await chip.boundingBox())!;
    expect(Math.abs(unwrapped.y - bounds.line.y)).toBeLessThan(
      bounds.chip.height / 2,
    );
    await input.press("Alt+z");
    await expect(input).toHaveAttribute("wrap", "soft");
    await expect(input).toHaveValue(value);
  });
}
