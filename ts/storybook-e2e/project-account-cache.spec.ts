import { expect, test, type Page } from "@playwright/test";

async function open(page: Page, story: string) {
  const time = new Date("2026-11-14T12:00:00Z");
  await page.clock.install({ time });
  await page.clock.pauseAt(new Date(time.getTime() + 60_000));
  await page.goto(`/iframe.html?id=pages-projects--${story}&viewMode=story`, {
    waitUntil: "domcontentloaded",
  });
  return page.getByRole("table", { name: "Sessions", exact: true });
}

test("cold account reads share a Nox effect and replace button-sized placeholders without moving rows", async ({
  page,
}) => {
  const table = await open(page, "accounts-loading");
  await expect(table.locator("tbody tr")).toHaveCount(2);
  const placeholders = table.locator('[data-skeleton="placeholder"]');
  await expect(placeholders).toHaveCount(2);
  await expect(table.locator('td[aria-busy="true"]')).toHaveCount(2);
  await expect(
    table.getByRole("link", { name: "Work account work", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByLabel("Page account requests")).toHaveText("1");
  await page.clock.runFor(100);
  const group = table.locator('xpath=ancestor::*[@data-skeleton="group"]');
  await expect(group.locator('[data-skeleton="effect"]')).toHaveCount(1);
  const before = await placeholders.evaluateAll((nodes) =>
    nodes.map((node) => {
      const { x, y, width, height } = node.getBoundingClientRect();
      return {
        x,
        y,
        width,
        height,
        radius: getComputedStyle(node).borderRadius,
      };
    }),
  );
  const rowsBefore = await table
    .locator("tbody tr")
    .evaluateAll((nodes) =>
      nodes.map((node) => node.getBoundingClientRect().height),
    );
  await page.clock.runFor(3200);
  const links = table.getByRole("link", {
    name: "Work account work",
    exact: true,
  });
  await expect(links).toHaveCount(2);
  await expect(placeholders).toHaveCount(0);
  await expect(table.locator('td[aria-busy="true"]')).toHaveCount(0);
  const after = await links.evaluateAll((nodes) =>
    nodes.map((node) => {
      const { x, y, width, height } = node.getBoundingClientRect();
      return {
        x,
        y,
        width,
        height,
        radius: getComputedStyle(node).borderRadius,
      };
    }),
  );
  expect(after).toEqual(before);
  expect(
    await table
      .locator("tbody tr")
      .evaluateAll((nodes) =>
        nodes.map((node) => node.getBoundingClientRect().height),
      ),
  ).toEqual(rowsBefore);
  await page
    .getByRole("button", { name: "Reopen project page", exact: true })
    .click();
  await expect(links).toHaveCount(2);
  await expect(placeholders).toHaveCount(0);
  await expect(page.getByLabel("Page account requests")).toHaveText("1");
});

test("a shared warm account cache paints names without a new request", async ({
  page,
}) => {
  const table = await open(page, "cached-accounts");
  await expect(
    table.getByRole("link", { name: "Work account work", exact: true }),
  ).toHaveCount(2);
  await expect(table.locator('[data-skeleton="placeholder"]')).toHaveCount(0);
  await expect(page.getByLabel("Page account requests")).toHaveText("0");
  await page
    .getByRole("button", { name: "Reopen project page", exact: true })
    .click();
  await expect(
    table.getByRole("link", { name: "Work account work", exact: true }),
  ).toHaveCount(2);
  await expect(page.getByLabel("Page account requests")).toHaveText("0");
});

test("expired cached account names stay visible during revalidation", async ({
  page,
}) => {
  const table = await open(page, "refreshing-accounts");
  const oldLinks = table.getByRole("link", {
    name: "Work account work",
    exact: true,
  });
  await expect(oldLinks).toHaveCount(2);
  await expect(page.getByLabel("Page account requests")).toHaveText("1");
  await expect(table.locator('[data-skeleton="placeholder"]')).toHaveCount(0);
  await expect(table.locator('td[aria-busy="true"]')).toHaveCount(0);
  const before = await oldLinks.first().boundingBox();
  await page.clock.runFor(3200);
  const updated = table.getByRole("link", {
    name: "Updated account work",
    exact: true,
  });
  await expect(updated).toHaveCount(2);
  expect(await updated.first().boundingBox()).toEqual(before);
  await expect(table.locator('[data-skeleton="placeholder"]')).toHaveCount(0);
});
