import type { Locator, Page } from "@playwright/test";

// Full source inspection is explicit; clicking a pasted chip only selects it.
export async function inspectPaste(page: Page, chip: Locator) {
  await chip.locator(".paste-chip-label").click();
  await page.keyboard.press("Control+p");
}
