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
