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
  await expect(page.getByText("4800", { exact: true })).toBeVisible();
});
