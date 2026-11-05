import { expect, test } from "@playwright/test";

test("top-right icon actions follow the requested order and Erase keeps its confirmation", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  await expect(actions).toBeVisible();
  const trigger = actions.getByRole("button", { name: "Erase", exact: true });
  const buttonBox = (await trigger.boundingBox())!;
  await expect(actions.locator('[data-section-layout="layout"]')).toHaveCount(
    0,
  );
  await expect(actions.locator('[data-section-layout="group"]')).toHaveCount(1);
  const sectionSurfaces = actions.locator('[data-section-layout="section"]');
  await expect(sectionSurfaces).toHaveCount(5);
  await expect(
    sectionSurfaces
      .first()
      .getByRole("heading", { name: "Actions", exact: true }),
  ).toHaveCount(0);
  await expect(sectionSurfaces.first()).toHaveJSProperty("tagName", "BUTTON");
  const order = ["Erase", "Remove", "Stop", "Start", "Refresh"];
  await expect
    .poll(() =>
      actions
        .getByRole("button")
        .evaluateAll((nodes) =>
          nodes.map((node) => node.getAttribute("aria-label")),
        ),
    )
    .toEqual(order);
  for (const name of order) {
    const action = actions.getByRole("button", { name, exact: true });
    await expect(action).toHaveText("");
    await expect(action.locator("svg")).toHaveCount(1);
    const box = (await action.boundingBox())!;
    expect(box.width).toBe(64);
    expect(box.height).toBe(box.width);
    expect(box.y).toBe(buttonBox.y);
    await expect(action.locator("h2, h3")).toHaveCount(0);
  }
  const info = page.getByRole("region", { name: "Information", exact: true });
  const usage = page.getByRole("region", {
    name: "Conversation usage",
    exact: true,
  });
  const infoBox = (await info.boundingBox())!,
    usageBox = (await usage.boundingBox())!;
  const refreshBox = (await actions
    .getByRole("button", { name: "Refresh", exact: true })
    .boundingBox())!;
  expect(buttonBox.y).toBe(infoBox.y);
  expect(buttonBox.x).toBeGreaterThan(infoBox.x + infoBox.width);
  expect(refreshBox.x + refreshBox.width).toBe(usageBox.x + usageBox.width);
  expect(usageBox.y).toBeGreaterThan(buttonBox.y + buttonBox.height);
  await expect(actions.getByRole("button")).toHaveCount(5);
  await expect(trigger).toHaveAttribute("data-section-layout", "section");
  await expect(trigger).toHaveAttribute("data-effect", "sparkle");
  await expect(trigger.locator("button")).toHaveCount(0);
  await trigger.hover();
  await expect(
    page.getByRole("tooltip", { name: "Erase", exact: true }),
  ).toBeVisible();
  await trigger.click();
  const dialog = page.getByRole("dialog", {
    name: "Erase project",
    exact: true,
  });
  await expect(dialog.locator(".floating-card")).toHaveCSS("opacity", "1");
  await expect(dialog.locator(".floating-card")).toHaveCSS("transform", "none");
  await expect(dialog).toContainText("This cannot be undone.");
  await expect(dialog).toContainText(
    "Workspace source files and shared accounts are preserved.",
  );
  await expect(dialog).toContainText("Web workspace");
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeFocused();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Web workspace");
  await trigger.click({
    position: { x: buttonBox.width - 4, y: buttonBox.height / 2 },
  });
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await trigger.click();
  await dialog
    .getByRole("button", { name: "Erase permanently", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toHaveText("API workspace");
  await expect(
    page
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(2);
  await picker.click();
  await expect(
    page.getByRole("option", { name: "Web workspace", exact: true }),
  ).toHaveCount(0);
  await page.keyboard.press("Escape");
  await page
    .getByRole("region", { name: "Actions", exact: true })
    .getByRole("button", { name: "Erase", exact: true })
    .click();
  await dialog
    .getByRole("button", { name: "Erase permanently", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByText("No projects yet.", { exact: true }),
  ).toBeVisible();
});

test("Refresh reloads the project without resetting session selection or layout", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/iframe.html?id=pages-projects--refreshing&viewMode=story");
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  const refresh = actions.getByRole("button", { name: "Refresh", exact: true });
  const usage = page.getByRole("region", {
    name: "Conversation usage",
    exact: true,
  });
  await expect(
    usage.getByRole("button", { name: "Refresh", exact: true }),
  ).toHaveCount(0);
  await expect(refresh).toBeEnabled();
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  const checkbox = table.locator("tbody tr").first().getByRole("checkbox");
  await checkbox.check();
  const before = await table.boundingBox();
  await refresh.click();
  await expect(refresh).toHaveAttribute("aria-busy", "true");
  await expect(refresh).toBeEnabled();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(checkbox).toBeChecked();
  expect(await table.boundingBox()).toEqual(before);
  expect(errors).toEqual([]);
});

test("Refresh failure is overlaid at its trigger and retains project details", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--refresh-failure&viewMode=story",
  );
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  const refresh = actions.getByRole("button", { name: "Refresh", exact: true });
  await expect(refresh).toBeEnabled();
  const before = await actions.boundingBox();
  await refresh.click();
  const error = page.getByRole("alert");
  await expect(error).toContainText("Project refresh is unavailable");
  await expect(
    page.getByRole("region", { name: "Information", exact: true }),
  ).toContainText("Web workspace");
  expect(await actions.boundingBox()).toEqual(before);
  await error
    .getByRole("button", { name: "Dismiss error", exact: true })
    .click();
  await expect(error).toHaveCount(0);
  await expect(refresh).toBeEnabled();
});

test("narrow project pages keep the icon actions at the top right without overflow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 760 });
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  const info = page.getByRole("region", { name: "Information", exact: true });
  const actionBox = (await actions.boundingBox())!,
    infoBox = (await info.boundingBox())!;
  expect(infoBox.y).toBeGreaterThan(actionBox.y + actionBox.height);
  const buttons = actions.getByRole("button");
  await expect(buttons).toHaveCount(5);
  const boxes = await buttons.evaluateAll((nodes) =>
    nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { x: box.x, y: box.y, width: box.width, height: box.height };
    }),
  );
  for (const box of boxes) {
    expect(box.width).toBeCloseTo(box.height, 1);
    expect(box.y).toBe(actionBox.y);
  }
  const last = boxes.at(-1)!;
  expect(last.x + last.width).toBeCloseTo(infoBox.x + infoBox.width, 1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    320,
  );
});

test("Remove confirms container removal while retaining sessions and the project", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  await actions.getByRole("button", { name: "Remove", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Remove container",
    exact: true,
  });
  await expect(dialog).toContainText(
    "sessions, conversation history and volumes are retained",
  );
  await dialog.getByRole("button", { name: "Remove", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(
    actions.getByRole("button", { name: "Remove", exact: true }),
  ).toBeDisabled();
  await expect(
    actions.getByRole("button", { name: "Start", exact: true }),
  ).toBeEnabled();
  await expect(
    page
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(2);
});

test("project erase failure retains the confirmation and selected project", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--unavailable&viewMode=story");
  await page
    .getByRole("region", { name: "Actions", exact: true })
    .getByRole("button", { name: "Erase", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Erase project",
    exact: true,
  });
  await dialog
    .getByRole("button", { name: "Erase permanently", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toContainText(
    "Project removal is unavailable",
  );
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeEnabled();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Web workspace");
  await expect(
    page
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(2);
});

test("Stop confirms, stops all project sessions and retains the project", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  const stop = actions.getByRole("button", { name: "Stop", exact: true });
  const rows = page
    .getByRole("table", { name: "Sessions", exact: true })
    .locator("tbody tr");
  await rows.first().getByRole("checkbox").check();
  await stop.click();
  const dialog = page.getByRole("dialog", {
    name: "Stop project",
    exact: true,
  });
  await expect(dialog).toContainText(
    "The project remains listed and can be started again.",
  );
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(stop).toBeEnabled();
  await expect(stop).toBeFocused();
  await stop.click();
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(stop).toBeDisabled();
  await expect(rows).toHaveCount(2);
  await expect
    .poll(() =>
      rows
        .getByRole("img")
        .evaluateAll((nodes) =>
          nodes.map((node) => node.getAttribute("aria-label")),
        ),
    )
    .toEqual(["Stopped", "Stopped"]);
  await expect(rows.first().getByRole("checkbox")).toBeChecked();
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toHaveText("Web workspace");
  await expect(
    actions.getByRole("button", { name: "Erase", exact: true }),
  ).toBeEnabled();
  await picker.click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(
    actions.getByRole("button", { name: "Stop", exact: true }),
  ).toBeEnabled();
  await expect
    .poll(() =>
      rows
        .getByRole("img")
        .evaluateAll((nodes) =>
          nodes.map((node) => node.getAttribute("aria-label")),
        ),
    )
    .toEqual(["Awaiting answer", "Idle"]);
  await picker.click();
  await page
    .getByRole("option", { name: "Web workspace", exact: true })
    .click();
  await expect(
    actions.getByRole("button", { name: "Stop", exact: true }),
  ).toBeDisabled();
});

test("failed project Stop retains the confirmation and records", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--unavailable&viewMode=story");
  await page
    .getByRole("region", { name: "Actions", exact: true })
    .getByRole("button", { name: "Stop", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Stop project",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText(
    "Project stop is unavailable",
  );
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(
    page
      .getByRole("region", { name: "Actions", exact: true })
      .getByRole("button", { name: "Stop", exact: true }),
  ).toBeEnabled();
  await expect(
    page
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(2);
});

test("Start asks for trust when the configuration needs it and starts only once given", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--requires-trust&viewMode=story",
  );
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  const start = actions.getByRole("button", { name: "Start", exact: true });
  const confirm = page.getByRole("dialog", {
    name: "Start project",
    exact: true,
  });
  const trust = page.getByRole("dialog", {
    name: "Trust configuration",
    exact: true,
  });
  await start.click();
  await confirm.getByRole("button", { name: "Start", exact: true }).click();
  await expect(confirm).toHaveCount(0);
  await expect(trust).toContainText(
    '"/services/dev/privileged": privileged container requested',
  );
  await expect(
    trust.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeFocused();
  await trust.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(trust).toHaveCount(0);
  await expect(start).toBeEnabled();
  await start.click();
  await confirm.getByRole("button", { name: "Start", exact: true }).click();
  await trust
    .getByRole("button", { name: "Trust and start", exact: true })
    .click();
  await expect(trust).toHaveCount(0);
  await expect(start).toBeDisabled();
  await expect(
    actions.getByRole("button", { name: "Stop", exact: true }),
  ).toBeEnabled();
});
