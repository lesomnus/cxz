import { test, expect } from "@playwright/test";
test("shared-memory story renders the live editor and read-only snapshots", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=resources-views--shared-memory&viewMode=story",
  );
  await page.getByRole("button", { name: "notes.md", exact: true }).click();
  await expect(page.locator(".monaco-editor")).toBeVisible();
  await page.getByRole("combobox", { name: "Memory", exact: true }).click();
  await page
    .getByRole("option", { name: "Saved checkpoint · Snapshot", exact: true })
    .click();
  await page.getByRole("button", { name: "notes.md", exact: true }).click();
  await expect(
    page.getByText("Read only. Fork this memory to edit it in this session.", {
      exact: true,
    }),
  ).toBeVisible();
});
test("account and usage stories use isolated typed clients", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  await expect(
    page.getByText("brokered-access-token", { exact: true }),
  ).toBeVisible();
  await page.goto("/iframe.html?id=resources-views--usage&viewMode=story");
  await expect(
    page.getByText("Complete retained coverage", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("img", { name: "4800", exact: true }),
  ).toBeVisible();
});

test("usage calendar shows daily details without an expander", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=resources-views--usage&viewMode=story");
  const activity = page.getByRole("region", {
    name: "Daily activity",
    exact: true,
  });
  await expect(activity.getByRole("img")).toHaveCount(30);
  await expect(page.locator("details")).toHaveCount(0);
  const busy = activity.getByRole("img", {
    name: "2026-10-24 (UTC): 4 finished turns",
    exact: true,
  });
  await busy.hover();
  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toContainText("2026-10-24 · UTC");
  await expect(
    tooltip.getByRole("img", { name: "800", exact: true }),
  ).toBeVisible();
  await expect(
    tooltip.getByRole("img", { name: "$0.14", exact: true }),
  ).toBeVisible();
  const quiet = activity.getByRole("img", {
    name: "2026-10-04 (UTC): 0 finished turns",
    exact: true,
  });
  await page.mouse.move(0, 0);
  await quiet.locator("..").focus();
  await expect(tooltip).toContainText("No recorded activity");
  await expect(tooltip).toContainText("2026-10-04 · UTC");
  await page.keyboard.press("Escape");
  await expect(tooltip).not.toBeVisible();
});
