import { expect, test } from "@playwright/test";
import { errorRowExamples } from "../src/shared/errors/error-row-examples";

for (const kind of ["all", "rpc", "http", "connection", "runtime", "client"])
  test(`error row catalog shows every registered ${kind} presentation and its details`, async ({
    page,
  }) => {
    await page.goto(
      `/iframe.html?id=workspace-errors-rows--${kind}&viewMode=story`,
    );
    const examples = errorRowExamples.filter(
      (item) => kind === "all" || item.context.origin?.kind === kind,
    );
    const table = page.getByRole("table", { name: "Errors", exact: true });
    await expect(
      table.getByRole("button", { name: "Details", exact: true }),
    ).toHaveCount(examples.length);
    const first = table
      .getByRole("button", { name: "Details", exact: true })
      .first();
    await first.click();
    await expect(
      page.getByRole("region", { name: "Error details", exact: true }),
    ).toContainText(examples.at(-1)!.message);
  });

for (const width of [1200, 390])
  test(`headerless error dialog has an outside usable scrollbar and no footer at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 850 });
    await page.goto(
      "/iframe.html?id=workspace-errors-rows--dialog&viewMode=story",
    );
    const card = page.locator(".floating-card");
    const table = page.getByRole("table", { name: "Errors", exact: true });
    await expect(table.locator("thead")).toHaveCount(0);
    await expect(card.locator(".card-footer")).toHaveCount(0);
    await expect(
      card.getByRole("button", { name: "Clear history", exact: true }),
    ).toHaveCount(0);
    const thumb = page.getByRole("scrollbar", {
      name: "Scroll errors",
      exact: true,
    });
    await expect(thumb).toBeVisible();
    // Card reveal must complete before checking actual hit targets outside it.
    await page.waitForTimeout(400);
    const thumbBox = (await thumb.boundingBox())!;
    const cardBox = (await card.boundingBox())!;
    const x = thumbBox.x + thumbBox.width / 2;
    expect(x).toBeGreaterThan(cardBox.x + cardBox.width);
    expect(x).toBeLessThan(width);
    expect(
      await page.evaluate(
        ({ x, y }) =>
          !!document.elementFromPoint(x, y)?.closest('[role="scrollbar"]'),
        { x, y: thumbBox.y + thumbBox.height / 2 },
      ),
    ).toBe(true);
    await page.mouse.move(x, thumbBox.y + thumbBox.height / 2);
    await page.mouse.down();
    await page.mouse.move(x, thumbBox.y + thumbBox.height / 2 + 100, {
      steps: 10,
    });
    await page.mouse.up();
    await expect(thumb).not.toHaveAttribute("aria-valuenow", "0");
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
  });

test("RPC details story exposes the full method while summary uses the readable operation", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=workspace-errors-rows--rpc-details&viewMode=story",
  );
  const details = page.getByRole("region", {
    name: "Error details",
    exact: true,
  });
  await expect(details).toContainText("cxz.SessionService/Transcript");
  await expect(details).toContainText("Read conversation");
});

for (const width of [1200, 390])
  test(`sidebar dialog uses two lines with operation left and time/count right at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 850 });
    await page.clock.install({ time: new Date("2026-11-07T09:00:00Z") });
    await page.goto(
      "/iframe.html?id=workspace-errors--recovery&viewMode=story",
    );
    await page
      .getByRole("button", { name: "Fail request", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Fail request", exact: true })
      .click();
    await page.clock.fastForward(65_000);
    await page.getByRole("button", { name: "Errors", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
    const table = dialog.getByRole("table", { name: "Errors", exact: true });
    const row = table.getByRole("row");
    await expect(row).toHaveCount(1);
    await expect(
      table.getByRole("button", { name: "Details", exact: true }),
    ).toHaveCount(0);
    await expect(table.getByRole("button")).toHaveCount(1);
    const message = row.getByTitle("Account service is offline", {
      exact: true,
    });
    const operation = row.getByTitle(/^Read Account · cxz.AccountService\/Get/);
    const time = row.locator("time");
    const count = row.getByTitle("2 occurrences", { exact: true });
    const dismiss = row.getByRole("button", { name: "Dismiss", exact: true });
    await expect(count).toContainText("002");
    const [m, o, d, c, x] = await Promise.all(
      [message, operation, time, count, dismiss].map((node) =>
        node.boundingBox(),
      ),
    );
    expect(o!.y).toBeGreaterThanOrEqual(m!.y + m!.height);
    expect(o!.x).toBeCloseTo(m!.x, 0);
    expect(o!.x + o!.width).toBeLessThanOrEqual(d!.x);
    expect(d!.x + d!.width).toBeLessThanOrEqual(c!.x);
    expect(c!.x + c!.width).toBeLessThanOrEqual(x!.x);
    expect(Math.abs(o!.y - d!.y)).toBeLessThan(3);
    expect(Math.abs(d!.y - c!.y)).toBeLessThan(3);
    await dismiss.click();
    await expect(table).toContainText("No active errors");
  });

const diagnosticCases = [
  {
    kind: "rpc",
    key: "cxz.SessionService/Transcript",
    values: ["NotFound", "Release review"],
  },
  { kind: "http", key: "POST /editor/connect", values: ["504", "web"] },
  {
    kind: "connection",
    key: "Provider login",
    values: ["WebSocket", "1006", "work"],
  },
  {
    kind: "runtime",
    key: "start-container",
    values: ["STOPPED", "cxz-web", "Web workspace"],
  },
  { kind: "client", key: "TypeError", values: ["conversation.js:42:7"] },
];
for (const { kind, key, values } of diagnosticCases)
  for (const dialog of [false, true])
    test(`${kind} ${dialog ? "dialog" : "page"} rows expose specific diagnostic evidence without opening Details`, async ({
      page,
    }) => {
      await page.goto(
        `/iframe.html?id=workspace-errors-rows--${kind}${dialog ? "-dialog" : ""}&viewMode=story`,
      );
      const table = page.getByRole("table", { name: "Errors", exact: true });
      const row = table.getByRole("row").filter({ hasText: key });
      await expect(row).toHaveCount(1);
      await row.scrollIntoViewIfNeeded();
      await expect(
        row.getByRole("region", { name: "Error details", exact: true }),
      ).toHaveCount(0);
      await expect(row).toContainText(key);
      for (const value of values) await expect(row).toContainText(value);
      if (dialog)
        await expect(
          row.getByRole("button", { name: "Details", exact: true }),
        ).toHaveCount(0);
    });
