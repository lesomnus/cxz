import { expect, test, type Locator } from "@playwright/test";

async function cells(activity: Locator) {
  return activity.getByRole("button").evaluateAll((nodes) =>
    nodes.map((node) => ({
      label: node.getAttribute("aria-label"),
      level: node.getAttribute("data-level"),
      selected: node.getAttribute("aria-pressed"),
    })),
  );
}

for (const kind of ["project", "account"] as const) {
  test(`${kind} refresh preserves calendar nodes and selection until new history arrives`, async ({
    page,
  }) => {
    await page.clock.install({ time: new Date("2026-11-01T18:00:00Z") });
    await page.clock.pauseAt(new Date("2026-11-01T18:00:01Z"));
    await page.goto(
      `/iframe.html?id=${kind === "project" ? "pages-projects--usage-refresh" : "accounts-cache--slow-refresh"}&viewMode=story`,
    );
    const root =
      kind === "project"
        ? page.getByRole("main")
        : page.locator('[data-account="work"]');
    const activity = root.getByRole("region", {
      name: "Daily activity",
      exact: true,
    });
    await expect(activity).toHaveAttribute("aria-busy", "true");
    await page.clock.runFor(650);
    await page.clock.runFor(650);
    await expect(activity).toHaveAttribute("aria-busy", "false");
    const date = "2026-10-31";
    const selected = activity.getByRole("button", {
      name: `${date} (UTC): 2 finished turns`,
      exact: true,
    });
    await selected.evaluate((node: HTMLButtonElement) => node.click());
    const node = await selected.elementHandle();
    const before = await cells(activity);
    const geometry = await activity.boundingBox();
    await activity.evaluate((element) => {
      Reflect.set(element, "removedCalendarButtons", 0);
      const observer = new MutationObserver((records) => {
        const removed = records
          .flatMap((record) => Array.from(record.removedNodes))
          .reduce(
            (count, node) =>
              count +
              (node instanceof Element
                ? node.tagName === "BUTTON"
                  ? 1
                  : node.querySelectorAll("button").length
                : 0),
            0,
          );
        Reflect.set(
          element,
          "removedCalendarButtons",
          Reflect.get(element, "removedCalendarButtons") + removed,
        );
      });
      observer.observe(element, { childList: true, subtree: true });
    });
    await page
      .getByRole("button", {
        name:
          kind === "project"
            ? "Change usage on server"
            : "Change account on server",
        exact: true,
      })
      .evaluate((button: HTMLButtonElement) => button.click());
    if (kind === "account")
      await expect(page.getByLabel("Server account name")).toHaveText(
        "Updated account",
      );
    const refresh =
      kind === "project"
        ? root
            .getByRole("region", { name: "Actions", exact: true })
            .getByRole("button", { name: "Refresh", exact: true })
        : root.getByRole("button", { name: /^Refresh account / });
    await refresh.evaluate((button: HTMLButtonElement) => button.click());
    await expect(activity).toHaveAttribute("aria-busy", "true");
    expect(await cells(activity)).toEqual(before);
    await page.clock.runFor(650);
    // Recent totals have arrived. The separate calendar read is still pending.
    await expect(activity).toHaveAttribute("aria-busy", "true");
    expect(await cells(activity)).toEqual(before);
    expect(await node!.evaluate((element) => element.isConnected)).toBe(true);
    await expect(activity.locator("[data-placeholder]")).toHaveCount(0);
    expect(await activity.boundingBox()).toEqual(geometry);
    await page.clock.runFor(650);
    await expect(activity).toHaveAttribute("aria-busy", "false");
    const updated = activity.getByRole("button", {
      name: `${date} (UTC): 4 finished turns`,
      exact: true,
    });
    await expect(updated).toHaveAttribute("aria-pressed", "true");
    expect(await node!.evaluate((element) => element.isConnected)).toBe(true);
    expect(
      await activity.evaluate((element) =>
        Reflect.get(element, "removedCalendarButtons"),
      ),
    ).toBe(0);
    expect(await activity.boundingBox()).toEqual(geometry);

    // Refresh again with an unchanged range: both reads must still bypass cache.
    await refresh.evaluate((button: HTMLButtonElement) => button.click());
    await page.clock.runFor(650);
    await expect(activity).toHaveAttribute("aria-busy", "true");
    await expect(updated).toHaveAttribute("aria-pressed", "true");
    await page.clock.runFor(650);
    await expect(activity).toHaveAttribute("aria-busy", "false");
    expect(
      await activity.evaluate((element) =>
        Reflect.get(element, "removedCalendarButtons"),
      ),
    ).toBe(0);

    if (kind === "project") {
      await page
        .getByRole("combobox", { name: "Select project", exact: true })
        .evaluate((button: HTMLElement) => button.click());
      await page
        .getByRole("option", { name: "API workspace", exact: true })
        .evaluate((button: HTMLElement) => button.click());
      await expect(activity).toHaveAttribute("aria-busy", "true");
      await expect(activity.getByRole("button")).toHaveCount(0);
      await page.clock.runFor(650);
      await page.clock.runFor(650);
      await expect(activity).toHaveAttribute("aria-busy", "false");
      await expect(
        activity.getByRole("button", {
          name: `${date} (UTC): 8 finished turns`,
          exact: true,
        }),
      ).toHaveAttribute("aria-pressed", "false");
    }
  });
}
