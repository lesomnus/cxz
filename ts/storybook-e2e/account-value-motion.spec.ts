import { expect, test, type Page } from "@playwright/test";

async function ready(page: Page) {
  const graphs = page.getByRole("region", {
    name: "Daily activity",
    exact: true,
  });
  await expect(graphs).toHaveCount(2);
  for (const graph of await graphs.all())
    await expect(graph).toHaveAttribute("aria-busy", "false");
}

async function readCount(page: Page, kind: string) {
  return Number(
    (await page.getByLabel(`${kind} reads`).innerText()).split(":")[1],
  );
}

test("account refresh immediately gates repeated clicks through the complete usage chain", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=accounts-cache--slow-refresh&viewMode=story",
  );
  await ready(page);
  const work = page.locator('[data-account="work"]');
  const refresh = work.getByRole("button", {
    name: "Refresh account Work",
    exact: true,
  });
  await expect(refresh).not.toHaveAttribute("data-loading");
  const before = {
    accounts: await readCount(page, "accounts"),
    usage: await readCount(page, "usage"),
  };
  await refresh.evaluate((button: HTMLButtonElement) => {
    button.click();
    button.click();
    button.click();
  });
  await expect(refresh).toHaveAttribute("aria-busy", "true");
  await expect(refresh).toBeDisabled();
  await refresh.press("Enter");
  await ready(page);
  await expect(refresh).not.toHaveAttribute("data-loading");
  expect(await readCount(page, "accounts")).toBe(before.accounts + 1);
  expect(await readCount(page, "usage")).toBe(before.usage + 2);
});

test("changed values outside the scroll viewport commit without motion and never replay on entry", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1000, height: 500 });
  await page.goto("/iframe.html?id=accounts-cache--default&viewMode=story");
  await ready(page);
  const work = page.locator('[data-account="work"]');
  const name = work
    .getByRole("button", { name: "Edit Display name", exact: true })
    .locator("[data-motion][aria-label]");
  await page
    .getByRole("article", { name: "Personal", exact: true })
    .evaluate((node) => node.scrollIntoView({ block: "start" }));
  await expect.poll(async () => (await name.boundingBox())!.y).toBeLessThan(0);
  await expect(name.locator("[data-changing]")).toHaveCount(0);
  await page.clock.install();
  const now = await page.evaluate(() => Date.now());
  await page.clock.pauseAt(new Date(now + 50));
  await page
    .getByRole("button", { name: "Change account on server", exact: true })
    .evaluate((node: HTMLButtonElement) => node.click());
  await expect(page.getByLabel("Server account name")).toHaveText(
    "Updated account",
  );
  await work
    .getByRole("button", { name: "Refresh account Work", exact: true })
    .evaluate((node: HTMLButtonElement) => node.click());
  await expect(name).toHaveAttribute("aria-label", "Updated account");
  await page.clock.runFor(32);
  await expect(name.locator("[data-changing]")).toHaveCount(0);
  await name.evaluate((node) => node.scrollIntoView({ block: "center" }));
  await page.clock.runFor(32);
  await expect(name).toBeVisible();
  await expect(name.locator("[data-changing]")).toHaveCount(0);
});

test("reduced motion keeps account formatting static", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=accounts-cache--default&viewMode=story");
  await ready(page);
  await expect(page.locator("[data-changing]")).toHaveCount(0);
  const work = page.locator('[data-account="work"]');
  await expect(
    work
      .getByRole("region", { name: "Recent 30 days", exact: true })
      .getByRole("img", { name: "4800", exact: true }),
  ).toBeVisible();
});

test("visible changed values slide from top left with bounded random character delays", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=accounts-cache--default&viewMode=story");
  await ready(page);
  await expect(page.locator("[data-changing]")).toHaveCount(0);
  const work = page.locator('[data-account="work"]');
  const information = work.getByRole("region", {
    name: "Account details",
    exact: true,
  });
  const field = (label: string) =>
    information
      .getByRole("term")
      .filter({ hasText: label })
      .locator("..")
      .getByRole("definition");
  const name = work
    .getByRole("button", { name: "Edit Display name", exact: true })
    .locator("[data-motion][aria-label]");
  const input = work
    .getByRole("region", { name: "Recent 30 days", exact: true })
    .getByRole("img", { name: "9600", exact: true })
    .locator("[data-motion][aria-label]");
  await page.clock.install();
  const now = await page.evaluate(() => Date.now());
  await page.clock.pauseAt(new Date(now + 50));
  await page
    .getByRole("button", { name: "Change account on server", exact: true })
    .evaluate((node: HTMLButtonElement) => node.click());
  await expect(page.getByLabel("Server account name")).toHaveText(
    "Updated account",
  );
  await work
    .getByRole("button", { name: "Refresh account Work", exact: true })
    .evaluate((node: HTMLButtonElement) => node.click());
  await expect(name).toHaveAttribute("aria-label", "Updated account");
  await expect(name.locator("[data-changing]").first()).toBeAttached();
  await expect(input.locator("[data-changing]").first()).toBeAttached();
  const delays = async (locator: import("@playwright/test").Locator) =>
    locator
      .locator("[data-changing]")
      .evaluateAll((nodes) =>
        nodes.map((node) =>
          parseFloat(
            getComputedStyle(node).getPropertyValue("--animated-text-delay"),
          ),
        ),
      );
  const nameDelays = await delays(name);
  const inputDelays = await delays(input);
  expect(new Set(nameDelays).size).toBeGreaterThan(1);
  expect(Math.max(...nameDelays) - Math.min(...nameDelays)).toBeLessThanOrEqual(
    90,
  );
  expect(Math.min(...nameDelays)).toBeLessThan(Math.min(...inputDelays));
  await expect(field("Created").locator("[data-changing]")).toHaveCount(0);
});
