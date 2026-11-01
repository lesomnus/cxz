import { expect, test, type Locator } from "@playwright/test";

async function panels(account: Locator) {
  return account
    .locator(":scope [data-section-layout=section]")
    .evaluateAll((nodes) =>
      nodes
        .filter(
          (node) =>
            node.tagName === "SECTION" && node.hasAttribute("aria-label"),
        )
        .map((node) => {
          const { x, y, width, height } = node.getBoundingClientRect();
          return {
            label: node.getAttribute("aria-label"),
            x,
            y,
            width,
            height,
          };
        }),
    );
}

for (const width of [1280, 390]) {
  test(`account panels reserve their loaded layout at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.clock.install({ time: new Date("2026-11-01T18:00:00Z") });
    await page.clock.pauseAt(new Date("2026-11-01T18:00:01Z"));
    await page.goto(
      "/iframe.html?id=resources-views--accounts-loading&viewMode=story",
    );
    const work = page.getByRole("article", { name: "Work", exact: true });
    const activity = work.getByRole("region", {
      name: "Daily activity",
      exact: true,
    });
    const usage = work.getByRole("region", {
      name: "Conversation usage",
      exact: true,
    });
    await expect(activity).toHaveAttribute("aria-busy", "true");
    await expect(activity.getByRole("button")).toHaveCount(0);
    const placeholders = activity.locator("[data-placeholder]");
    expect(await placeholders.count()).toBeGreaterThan(100);
    await expect(
      usage.getByRole("img", { name: "Unavailable", exact: true }),
    ).toHaveCount(16);
    const pending = await panels(work);
    const boxes = await placeholders.count();

    await page.clock.fastForward(1500);
    await expect(
      usage
        .getByRole("region", { name: "Recent 30 days", exact: true })
        .getByRole("img", { name: "4800", exact: true }),
    ).toBeVisible();
    await page.clock.fastForward(1500);
    await expect(activity).toHaveAttribute("aria-busy", "false");
    await expect(placeholders).toHaveCount(0);
    expect(await activity.locator("[data-date]").count()).toBe(boxes);
    expect(await panels(work)).toEqual(pending);
    await expect(
      work
        .getByRole("table", { name: "Sessions", exact: true })
        .locator("tbody tr"),
    ).toHaveCount(2);

    await usage.getByRole("button", { name: "Refresh", exact: true }).click();
    await expect(activity).toHaveAttribute("aria-busy", "true");
    expect(await panels(work)).toEqual(pending);
    await page.clock.fastForward(1500);
    await page.clock.fastForward(1500);
    await expect(activity).toHaveAttribute("aria-busy", "false");
    expect(await panels(work)).toEqual(pending);
  });

  test(`long account fields preserve panel heights at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
    const work = page.locator('[data-account="work"]');
    await expect(
      work.getByRole("region", { name: "Daily activity", exact: true }),
    ).toHaveAttribute("aria-busy", "false");
    const normal = await panels(work);
    await page.goto(
      "/iframe.html?id=resources-views--accounts-long-details&viewMode=story",
    );
    await expect(
      work.getByRole("region", { name: "Daily activity", exact: true }),
    ).toHaveAttribute("aria-busy", "false");
    expect(await panels(work)).toEqual(normal);
    const name = work
      .getByRole("region", { name: "Account details", exact: true })
      .getByText(
        "Work account with a very long display name that must not resize the panel",
        { exact: true },
      );
    expect(await name.evaluate((el) => el.scrollWidth > el.clientWidth)).toBe(
      true,
    );
    await name.hover();
    await expect(page.getByRole("tooltip")).toContainText(
      "Work account with a very long display name",
    );
    expect(
      await page
        .locator("body")
        .evaluate((el) => el.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });
}

test("usage error and retry keep the calendar and metric shells in place", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=resources-views--usage-failure&viewMode=story",
  );
  const usage = page.getByRole("region", {
    name: "Conversation usage",
    exact: true,
  });
  await expect(usage.getByRole("alert")).toBeVisible();
  const failed = await usage.boundingBox();
  await expect(
    usage
      .getByRole("region", { name: "Daily activity", exact: true })
      .getByRole("button"),
  ).toHaveCount(0);
  await usage.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(
    usage.getByRole("region", { name: "Daily activity", exact: true }),
  ).toHaveAttribute("aria-busy", "false");
  await expect(
    usage
      .getByRole("region", { name: "Recent 30 days", exact: true })
      .getByRole("img", { name: "4800", exact: true }),
  ).toBeVisible();
  expect(await usage.boundingBox()).toEqual(failed);
  await expect(usage.getByRole("alert")).toHaveCount(0);
});
