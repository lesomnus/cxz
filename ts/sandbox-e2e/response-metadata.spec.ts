import { expect, test } from "@playwright/test";

test("response headings keep their own model and effort after settings change", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  const headings = page.locator(".response-settings");
  await expect(headings).toHaveText(["sandbox-claude · high"], {
    timeout: 45000,
  });
  const originalSeq = await headings
    .first()
    .evaluate((el) => el.closest("article")!.dataset.seq!);
  const original = page.locator(
    `article[data-seq="${originalSeq}"] .response-settings`,
  );
  const pane = page.locator(".transcript");
  await page.getByRole("combobox", { name: "Effort", exact: true }).click();
  await page.getByRole("option", { name: "low", exact: true }).click();
  await expect(page.locator(".effort-field .meta-value")).toHaveText("low");
  await page.getByRole("combobox", { name: "Model", exact: true }).click();
  await page
    .getByRole("option", { name: "sandbox-claude-compact", exact: true })
    .click();
  await expect(page.locator(".model-field .meta-value")).toHaveText(
    "sandbox-claude-compact",
  );
  await expect(page.locator(".effort-field .meta-value")).toHaveText("high");
  await page.getByRole("combobox", { name: "Effort", exact: true }).click();
  await page.getByRole("option", { name: "low", exact: true }).click();
  await expect(page.locator(".effort-field .meta-value")).toHaveText("low");
  await expect(headings).toHaveText(["sandbox-claude · high"]);
  await page
    .getByRole("textbox", { name: "Message", exact: true })
    .fill("Snapshot preview");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(headings.last()).toHaveText("sandbox-claude-compact · low");
  await expect(headings.last()).toHaveAttribute("title", /Applied setting/);
  // Earlier rows may be unmounted by virtualization once the new turn finishes.
  await expect(page.locator(".conversation")).toHaveAttribute(
    "aria-description",
    /idle/,
  );
  await pane.evaluate((el) => (el.scrollTop = 0));
  await expect(original).toHaveText("sandbox-claude · high");
  const alignment = await original.evaluate((el) => {
    const logo = el
      .parentElement!.querySelector(".agent-brand")!
      .getBoundingClientRect();
    const text = el.getBoundingClientRect();
    return Math.abs(logo.y + logo.height / 2 - text.y - text.height / 2);
  });
  expect(alignment).toBeLessThan(0.1);
  // Reopening loads the same snapshots through History rather than current values.
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(headings).toHaveText(["sandbox-codex · high"]);
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-1");
  await expect(headings.last()).toHaveText("sandbox-claude-compact · low");
  await pane.evaluate((el) => (el.scrollTop = 0));
  await expect(original).toHaveText("sandbox-claude · high");
  await page.screenshot({
    path: "test-results/response-snapshots.png",
    fullPage: true,
  });
});
