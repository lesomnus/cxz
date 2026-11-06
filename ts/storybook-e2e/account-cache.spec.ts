import { expect, test } from "@playwright/test";

async function counts(page: import("@playwright/test").Page) {
  return Object.fromEntries(
    await Promise.all(
      ["accounts", "auth", "usage", "quota"].map(async (kind) => [
        kind,
        Number(
          (await page.getByLabel(`${kind} reads`).innerText()).split(":")[1],
        ),
      ]),
    ),
  );
}

async function loaded(page: import("@playwright/test").Page) {
  await expect(
    page.getByRole("region", { name: "Daily activity", exact: true }),
  ).toHaveCount(2);
  for (const activity of await page
    .getByRole("region", { name: "Daily activity", exact: true })
    .all())
    await expect(activity).toHaveAttribute("aria-busy", "false");
}

test("account and usage reads respect their separate TTLs when revisiting", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=accounts-cache--default&viewMode=story");
  await loaded(page);
  const start = await page.evaluate(() => Date.now());
  await page.clock.setFixedTime(new Date(start));
  const initial = await counts(page);
  expect(initial.accounts).toBe(1);
  expect(initial.usage).toBe(4);
  await page
    .getByRole("button", { name: "Leave accounts", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Open accounts", exact: true })
    .click();
  await loaded(page);
  expect(await counts(page)).toEqual(initial);
  await page
    .getByRole("button", { name: "Leave accounts", exact: true })
    .click();
  await page.clock.setFixedTime(new Date(start + 61_000));
  await page
    .getByRole("button", { name: "Open accounts", exact: true })
    .click();
  await loaded(page);
  const minute = await counts(page);
  expect(minute.accounts).toBe(initial.accounts);
  expect(minute.auth).toBe(initial.auth);
  expect(minute.usage).toBe(initial.usage + 4);
  expect(minute.quota).toBe(initial.quota + 1);
  await page
    .getByRole("button", { name: "Leave accounts", exact: true })
    .click();
  await page.clock.setFixedTime(new Date(start + 302_000));
  await page
    .getByRole("button", { name: "Open accounts", exact: true })
    .click();
  await loaded(page);
  const fiveMinutes = await counts(page);
  expect(fiveMinutes.accounts).toBe(initial.accounts + 1);
  expect(fiveMinutes.auth).toBe(initial.auth + 2);
  expect(fiveMinutes.usage).toBe(minute.usage + 4);
});

test("the square action after Erase bypasses caches for metadata, auth, quota and both usage reads", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=accounts-cache--default&viewMode=story");
  await loaded(page);
  const initial = await counts(page);
  const work = page.getByRole("article", { name: "Work", exact: true });
  const refresh = work.getByRole("button", {
    name: "Refresh account Work",
    exact: true,
  });
  const erase = work.getByRole("button", {
    name: "Erase account Work",
    exact: true,
  });
  const box = (await refresh.boundingBox())!;
  const erased = (await erase.boundingBox())!;
  expect(box.width).toBe(64);
  expect(box.height).toBe(64);
  expect(box.x).toBeGreaterThan(erased.x + erased.width);
  expect(box.y).toBe(erased.y);
  await expect(
    work
      .getByRole("region", { name: "Conversation usage", exact: true })
      .getByRole("button", { name: "Refresh", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Change account on server", exact: true })
    .click();
  await expect(page.getByLabel("Server account name")).toHaveText(
    "Updated account",
  );
  await expect(work).toBeVisible();
  await refresh.click();
  await expect(
    page.getByRole("article", { name: "Updated account", exact: true }),
  ).toBeVisible();
  await loaded(page);
  const updated = await counts(page);
  expect(updated.accounts).toBe(initial.accounts + 1);
  expect(updated.auth).toBe(initial.auth + 1);
  expect(updated.quota).toBe(initial.quota + 1);
  expect(updated.usage).toBe(initial.usage + 2);
});
