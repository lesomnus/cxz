import type { Locator, Page } from "@playwright/test";

// Click the chip body rather than its Text/File delivery controls.
export async function inspectPaste(_page: Page, chip: Locator) {
  await chip.locator(".paste-chip-label").click();
}
