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
    page.getByRole("region", { name: "Recent 30 days", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("img", { name: "4800", exact: true }),
  ).toBeVisible();
});

test("usage calendar selects daily statistics without a popover", async ({
  page,
}) => {
  await page.clock.setFixedTime(new Date("2026-11-01T18:00:00Z"));
  await page.goto("/iframe.html?id=resources-views--usage&viewMode=story");
  const activity = page.getByRole("region", {
    name: "Daily activity",
    exact: true,
  });
  const recent = page.getByRole("region", {
    name: "Recent 30 days",
    exact: true,
  });
  const daily = page.getByRole("region", {
    name: "Daily statistics",
    exact: true,
  });
  await expect(daily.getByText("2026-11-01", { exact: true })).toBeVisible();
  await expect(daily.getByText("today", { exact: true })).toBeVisible();
  await expect(daily.getByRole("button")).toHaveCount(0);
  await expect(
    daily.getByRole("img", { name: "200", exact: true }),
  ).toBeVisible();
  await expect(
    recent.getByRole("img", { name: "4800", exact: true }),
  ).toBeVisible();
  await expect(page.locator("details")).toHaveCount(0);
  const busy = activity.getByRole("button", {
    name: "2026-10-24 (UTC): 4 finished turns",
    exact: true,
  });
  await busy.hover();
  await expect(page.getByRole("tooltip")).not.toBeVisible();
  const calendarTop = (await activity.boundingBox())!.y;
  await busy.click();
  expect((await activity.boundingBox())!.y).toBeCloseTo(calendarTop, 0);
  await expect(busy).toHaveAttribute("aria-pressed", "true");
  await expect(daily.getByText("2026-10-24", { exact: true })).toBeVisible();
  await expect(daily.locator("time")).toHaveAttribute(
    "aria-label",
    "8 days ago",
  );
  await expect(
    daily.locator("time").getByRole("img", { name: "8", exact: true }),
  ).toHaveAttribute("data-mode", "integer");
  await expect(
    daily.getByRole("img", { name: "800", exact: true }),
  ).toBeVisible();
  await expect(
    daily.getByRole("img", { name: "$0.14", exact: true }),
  ).toBeVisible();
  // The wider calendar's August activity does not change the rolling total.
  const older = activity.getByRole("button", {
    name: "2026-08-12 (UTC): 6 finished turns",
    exact: true,
  });
  await older.focus();
  await page.keyboard.press("Enter");
  await expect(daily.getByText("2026-08-12", { exact: true })).toBeVisible();
  await expect(
    daily.getByRole("img", { name: "900", exact: true }),
  ).toBeVisible();
  await expect(
    recent.getByRole("img", { name: "4800", exact: true }),
  ).toBeVisible();
  await expect(daily.getByRole("button")).toHaveCount(0);
  await activity
    .getByRole("button", {
      name: "2026-11-01 (UTC): 1 finished turns",
      exact: true,
    })
    .click();
  await expect(daily.getByText("2026-11-01", { exact: true })).toBeVisible();
  await expect(
    daily.getByRole("img", { name: "200", exact: true }),
  ).toBeVisible();
  const quiet = activity.getByRole("button", {
    name: "2026-10-04 (UTC): 0 finished turns",
    exact: true,
  });
  await quiet.click();
  await expect(daily.getByText("2026-10-04", { exact: true })).toBeVisible();
  await expect(daily.getByRole("img", { name: "0", exact: true })).toHaveCount(
    7,
  );
  await expect(
    daily.getByRole("img", { name: "Not reported", exact: true }),
  ).toHaveText("-");
});

for (const width of [390, 600]) {
  test(`usage calendar fills a ${width}px panel and adjusts its time range`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/iframe.html?id=resources-views--usage&viewMode=story");
    const activity = page.getByRole("region", {
      name: "Daily activity",
      exact: true,
    });
    const oldest = activity.getByRole("button").first();
    await expect(oldest).toBeVisible();
    await expect(activity).toHaveAttribute("aria-busy", "false");
    const before = (await oldest.getAttribute("aria-label"))!;
    const geometry = await activity.evaluate((el) => {
      const buttons = [...el.querySelectorAll("button")];
      const last = buttons.at(-1)!.getBoundingClientRect();
      const grid =
        buttons[0].parentElement!.parentElement!.getBoundingClientRect();
      const first = buttons[0].getBoundingClientRect();
      return {
        right: last.right,
        edge: grid.right,
        first: first.left,
        start: grid.left,
        size: first.width,
        overflow: el.scrollWidth > el.clientWidth,
      };
    });
    expect(geometry.size).toBe(12);
    expect(Math.abs(geometry.right - geometry.edge)).toBeLessThan(1);
    expect(Math.abs(geometry.first - geometry.start)).toBeLessThan(1);
    expect(geometry.overflow).toBe(false);
    await page.setViewportSize({
      width: width === 390 ? 600 : 390,
      height: 900,
    });
    await expect(oldest).not.toHaveAttribute("aria-label", before);
    await expect(
      page
        .getByRole("region", { name: "Recent 30 days", exact: true })
        .getByRole("img", { name: "4800", exact: true }),
    ).toBeVisible();
  });
}

test("activity cells alternate black month fills and show hollow future days", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=resources-views--usage&viewMode=story");
  const activity = page.getByRole("region", {
    name: "Daily activity",
    exact: true,
  });
  const october = activity.locator('[data-date="2026-10-25"]');
  await expect(october).toHaveAttribute("data-month", "2026-10");
  const november = activity.locator('[data-date="2026-11-01"]');
  await expect(november).toHaveAttribute("data-month-tone", "dim");
  const futures = activity.locator('[data-future="true"]');
  await expect(futures).toHaveCount(6);
  await expect(
    activity.locator('[data-date="2026-11-07"] [data-future]'),
  ).toBeVisible();
  for (const cell of await futures.all()) {
    await expect(cell).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await expect(cell).toHaveCSS("border-top-color", "rgb(16, 16, 16)");
    await expect(cell).toHaveCSS("border-top-width", "1px");
    await expect(cell).toHaveCSS("width", "12px");
    await expect(cell).toHaveCSS("height", "12px");
    expect(await cell.evaluate((el) => el.tagName)).toBe("SPAN");
  }
  const tones = await activity.evaluate((el) => {
    const color = (date: string) =>
      getComputedStyle(el.querySelector(`[data-date="${date}"] button`)!)
        .backgroundColor;
    return [color("2026-07-01"), color("2026-08-01"), color("2026-09-01")];
  });
  expect(tones[0]).toBe("rgb(0, 0, 0)");
  expect(tones[1]).not.toBe(tones[0]);
  expect(tones[2]).toBe(tones[0]);
  for (const [date, color] of [
    ["2026-10-24", tones[1]],
    ["2026-11-01", tones[0]],
  ]) {
    const active = activity.locator(`[data-date="${date}"] button`);
    await expect(active).toHaveCSS("border-top-color", color);
    await expect(active).toHaveCSS("border-top-width", "1px");
    expect(
      await active.evaluate((el) => getComputedStyle(el).backgroundColor),
    ).not.toBe(color);
  }
  await expect(
    activity.getByRole("button", { name: /2026-11-0[2-7]/ }),
  ).toHaveCount(0);
  const oldDate = activity.getByRole("button", {
    name: "2026-10-24 (UTC): 4 finished turns",
    exact: true,
  });
  await oldDate.click();
  const daily = page.getByRole("region", {
    name: "Daily statistics",
    exact: true,
  });
  await futures.first().click();
  await expect(daily.getByText("2026-10-24", { exact: true })).toBeVisible();
});
