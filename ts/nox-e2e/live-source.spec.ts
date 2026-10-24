import { test, expect } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";

test("Nox TSX and CSS edits reach the consumer without rebuilding or losing state", async ({
  page,
}) => {
  const component = "packages/nox/src/components/button/button.tsx";
  const tokens = "packages/nox/src/theme/tokens.css";
  const originalComponent = readFileSync(component, "utf8");
  const originalTokens = readFileSync(tokens, "utf8");
  const changedComponent = originalComponent.replace(
    "<button\n",
    '<button\n      data-nox-live="updated"\n',
  );
  const changedTokens = `${originalTokens}\n:root { --ui-button-hover-background: rgb(12, 34, 56); }\n`;
  expect(changedComponent).not.toBe(originalComponent);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  try {
    await page.goto("/test-support/nox-development.html");
    const button = page.getByRole("button");
    await button.click();
    await expect(button).toHaveText("Count 1");
    const lifetime = await page.evaluate(() => performance.timeOrigin);
    writeFileSync(tokens, changedTokens);
    await button.hover();
    await expect(button).toHaveCSS("background-color", "rgb(12, 34, 56)");
    writeFileSync(component, changedComponent);
    await expect(button).toHaveAttribute("data-nox-live", "updated");
    await expect(button).toHaveText("Count 1");
    expect(await page.evaluate(() => performance.timeOrigin)).toBe(lifetime);
    expect(errors).toEqual([]);
  } finally {
    // Do not overwrite another editor's change during a development run.
    if (readFileSync(component, "utf8") === changedComponent)
      writeFileSync(component, originalComponent);
    if (readFileSync(tokens, "utf8") === changedTokens)
      writeFileSync(tokens, originalTokens);
  }
});
