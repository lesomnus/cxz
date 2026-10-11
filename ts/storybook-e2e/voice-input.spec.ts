import { expect, test } from "@playwright/test";

test("composer voice input and portable waveform examples are available", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-composer--voice-input&viewMode=story",
  );
  await expect(
    page.getByRole("combobox", { name: "Voice language" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Voice input", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible();
  await page.goto(
    "/iframe.html?id=components-audiowaveform--listening&viewMode=story",
  );
  await expect(page.locator("canvas[data-active=true]")).toBeVisible();
  expect(errors).toEqual([]);
});
