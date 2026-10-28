import { expect, test } from "@playwright/test";

test("native paste uses inline, Text and File defaults at the character boundaries", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const chip = page.locator(".paste-chip");
  for (const [body, mode] of [
    ["a".repeat(32), "inline"],
    ["one\ntwo\nthree\nfour", "inline"],
    ["😀".repeat(32), "inline"],
    ["a".repeat(33), "text"],
    ["한".repeat(64), "text"],
    ["😀".repeat(65), "file"],
  ] as const) {
    await input.fill("");
    await page.evaluate((text) => navigator.clipboard.writeText(text), body);
    await input.press("Control+v");
    if (mode === "inline") {
      await expect(input).toHaveValue(body);
      await expect(chip).toHaveCount(0);
    } else {
      await expect(chip).toHaveAttribute("data-send-as", mode);
      if (mode === "file")
        await expect(chip).toHaveAttribute("data-upload-state", "ready");
      else {
        await expect(chip).not.toHaveAttribute("data-upload-state");
        await expect(
          page.getByRole("button", { name: "Send", exact: true }),
        ).toBeEnabled();
      }
      await expect(
        chip.getByRole("radio", {
          name: mode === "text" ? "Text" : "File",
          exact: true,
        }),
      ).toBeChecked();
    }
  }
});

test("a medium paste uploads only after selecting File", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const body = "text first, optional attachment later";
  await page.evaluate((text) => navigator.clipboard.writeText(text), body);
  await input.press("Control+v");
  const chip = page.locator(".paste-chip");
  await expect(chip).toHaveAttribute("data-send-as", "text");
  await expect(chip).not.toHaveAttribute("data-upload-state");
  await chip.getByRole("radio", { name: "File", exact: true }).check();
  await expect(chip).toHaveAttribute("data-send-as", "file");
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await chip.getByRole("radio", { name: "Text", exact: true }).check();
  await expect(chip).toHaveAttribute("data-send-as", "text");
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toBeEnabled();
});
