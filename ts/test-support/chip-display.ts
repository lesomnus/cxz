import { expect, type Locator } from "@playwright/test";

export function readChipDisplay(input: Locator) {
  return input.evaluate((el: HTMLTextAreaElement) => {
    const chips = Array.from(
      el
        .closest(".composer-editor")!
        .querySelectorAll("[data-chip-reservation] .paste-chip-label"),
      (node) => node.textContent ?? "",
    );
    let index = 0;
    return el.value.replace(
      /\u200b([^\u200b]*)\u200b/g,
      () => chips[index++] ?? "",
    );
  });
}

export async function expectChipDisplay(input: Locator, expected: string) {
  await expect.poll(() => readChipDisplay(input)).toBe(expected);
}
