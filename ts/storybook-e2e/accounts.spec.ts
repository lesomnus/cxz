import { expect, test } from "@playwright/test";

test("account erase confirms the full cascade and cancel keeps the account", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  const work = page.getByRole("article", { name: "Work", exact: true });
  const erase = work.getByRole("button", {
    name: "Erase account Work",
    exact: true,
  });
  await erase.click();
  const modal = page.getByRole("dialog", {
    name: "Erase account · Work",
    exact: true,
  });
  await expect(modal).toContainText("all sessions linked to this account");
  await expect(modal).toContainText(
    "conversations, memory, attachments and agent profiles",
  );
  await expect(
    modal.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeFocused();
  await modal.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(modal).toHaveCount(0);
  await expect(work).toBeVisible();
  await erase.click();
  await modal
    .getByRole("button", { name: "Erase permanently", exact: true })
    .click();
  await expect(modal).toHaveCount(0);
  await expect(work).toHaveCount(0);
  await expect(
    page.getByRole("article", { name: "Personal", exact: true }),
  ).toBeVisible();
});

test("account erase cleanup failure stays visible and can be retried", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=resources-views--account-erase-failure&viewMode=story",
  );
  const work = page.getByRole("article", { name: "Work", exact: true });
  await work
    .getByRole("button", { name: "Erase account Work", exact: true })
    .click();
  const modal = page.getByRole("dialog", {
    name: "Erase account · Work",
    exact: true,
  });
  await modal
    .getByRole("button", { name: "Erase permanently", exact: true })
    .click();
  await expect(modal.getByRole("alert")).toContainText(
    "Session cleanup failed",
  );
  await expect(work).toBeVisible();
  await modal
    .getByRole("button", { name: "Erase permanently", exact: true })
    .click();
  await expect(modal).toHaveCount(0);
  await expect(work).toHaveCount(0);
});

test("account erase uses the immutable account id when available", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=accounts-workflows--account-login&viewMode=story",
  );
  const account = page.getByRole("article", {
    name: "Work account",
    exact: true,
  });
  await account
    .getByRole("button", { name: "Erase account Work account", exact: true })
    .click();
  const modal = page.getByRole("dialog", {
    name: "Erase account · Work account",
    exact: true,
  });
  await modal
    .getByRole("button", { name: "Erase permanently", exact: true })
    .click();
  await expect(modal).toHaveCount(0);
  await expect(
    page.getByText("No accounts yet.", { exact: true }),
  ).toBeVisible();
});

for (const width of [1280, 390]) {
  test(`account columns adapt without a sidebar at ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 800 });
    await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
    await expect(
      page.getByRole("complementary", { name: "Account list" }),
    ).toHaveCount(0);
    const work = page.getByRole("article", { name: "Work", exact: true });
    const personal = page.getByRole("article", {
      name: "Personal",
      exact: true,
    });
    await expect(work).toBeVisible();
    const layout = page.locator('[data-section-layout="layout"]');
    await expect(layout).toHaveCount(1);
    const group = work.locator('[data-section-layout="group"]').first();
    const sections = group.locator(':scope > [data-section-layout="section"]');
    await expect(sections).toHaveCount(2);
    const corners = await sections.evaluateAll((nodes) =>
      nodes.map((node) => {
        const css = getComputedStyle(node);
        return [css.borderTopLeftRadius, css.borderBottomLeftRadius];
      }),
    );
    expect(parseFloat(corners[0][0])).toBeGreaterThan(
      parseFloat(corners[0][1]),
    );
    expect(corners.at(-1)![1]).toBe(corners[0][0]);
    const usage = work.getByRole("region", {
      name: "Conversation usage",
      exact: true,
    });
    const sessions = work.getByRole("region", {
      name: "Sessions using this account",
      exact: true,
    });
    await expect(
      usage.getByRole("region", { name: "Recent 30 days", exact: true }),
    ).toBeVisible();
    const informationBounds = (await group.boundingBox())!;
    const usageBounds = (await usage.boundingBox())!;
    const sessionsBounds = (await sessions.boundingBox())!;
    const authBounds = (await work
      .getByRole("region", { name: "Authentication", exact: true })
      .boundingBox())!;
    expect(sessionsBounds.x).toBe(informationBounds.x);
    expect(sessionsBounds.width).toBe(informationBounds.width);
    const infoBounds = (await work
      .getByRole("region", { name: "Account details", exact: true })
      .boundingBox())!;
    expect(sessionsBounds.y).toBeGreaterThanOrEqual(
      infoBounds.y + infoBounds.height,
    );
    expect(usageBounds.y).toBeGreaterThanOrEqual(
      authBounds.y + authBounds.height,
    );
    const scrollBounds = (await sessions
      .getByRole("region", {
        name: "Session list",
        exact: true,
      })
      .boundingBox())!;
    const sessionsPadding = await sessions.evaluate((node) =>
      parseFloat(getComputedStyle(node).paddingBottom),
    );
    expect(scrollBounds.y + scrollBounds.height).toBeCloseTo(
      sessionsBounds.y + sessionsBounds.height - sessionsPadding,
      0,
    );
    const headerGap = await sessions.evaluate((node) =>
      parseFloat(
        getComputedStyle(node).getPropertyValue("--ui-section-group-gap"),
      ),
    );
    expect(scrollBounds.y - sessionsBounds.y).toBe(headerGap);
    await expect(sessions.getByRole("heading")).toHaveCount(0);
    const eraseBounds = (await work
      .getByRole("button", { name: "Erase account Work", exact: true })
      .boundingBox())!;
    expect(eraseBounds.width).toBe(64);
    expect(eraseBounds.height).toBe(64);
    await expect(
      work.getByRole("button", { name: "Erase account Work", exact: true }),
    ).toHaveCSS("border-radius", "16px");
    expect(authBounds.height).toBe(64);
    expect(authBounds.x + authBounds.width).toBeLessThan(eraseBounds.x);
    const refreshBounds = (await work
      .getByRole("button", {
        name: "Refresh account Work",
        exact: true,
      })
      .boundingBox())!;
    expect(refreshBounds.width).toBe(64);
    expect(refreshBounds.height).toBe(64);
    expect(eraseBounds.x + eraseBounds.width).toBeLessThan(refreshBounds.x);
    expect(refreshBounds.y).toBe(authBounds.y);
    expect(refreshBounds.x + refreshBounds.width).toBe(
      usageBounds.x + usageBounds.width,
    );
    expect(eraseBounds.y).toBe(authBounds.y);
    const quota = work.getByRole("region", {
      name: "Remaining agent usage",
      exact: true,
    });
    const quotaBounds = (await quota.boundingBox())!;
    await expect(quota).toHaveCSS("border-radius", "16px");
    expect(quotaBounds.width).toBe(64);
    expect(quotaBounds.height).toBe(64);
    expect(quotaBounds.y).toBe(authBounds.y);
    expect(quotaBounds.x).toBeGreaterThan(authBounds.x + authBounds.width);
    expect(quotaBounds.x + quotaBounds.width).toBeLessThan(eraseBounds.x);
    const gauge = quota.getByRole("meter").locator("..");
    const gaugeBounds = (await gauge.boundingBox())!;
    expect(gaugeBounds.width).toBe(32);
    expect(gaugeBounds.height).toBe(60);
    expect(gaugeBounds.x - quotaBounds.x).toBe(2);
    expect(gaugeBounds.y - quotaBounds.y).toBe(2);
    expect(
      quotaBounds.y + quotaBounds.height - gaugeBounds.y - gaugeBounds.height,
    ).toBe(2);
    expect(
      await gauge.evaluate((node) => getComputedStyle(node).borderRadius),
    ).toBe("14px");
    expect(gaugeBounds.x + 14).toBe(quotaBounds.x + 16);
    await expect(quota.getByRole("meter")).toHaveAttribute(
      "aria-valuenow",
      "64",
    );
    const empty = gauge.locator("[data-quota-vessel]");
    const cap = empty.locator("[data-quota-empty]");
    const contour = cap.locator("[data-quota-contour]");
    await expect
      .poll(() =>
        contour.evaluate((node) => (node as SVGPathElement).getBBox().height),
      )
      .toBeCloseTo(58 * 0.36, 0);
    const emptyBounds = (await empty.boundingBox())!;
    expect(emptyBounds.x - gaugeBounds.x).toBe(1);
    expect(emptyBounds.y - gaugeBounds.y).toBe(1);
    expect(
      gaugeBounds.x + gaugeBounds.width - emptyBounds.x - emptyBounds.width,
    ).toBe(1);
    expect(emptyBounds.height).toBe(58);
    await expect(empty).toHaveCSS("border-radius", "13px");
    const capBounds = (await cap.boundingBox())!;
    expect(capBounds.width).toBe(30);
    expect(capBounds.height).toBeCloseTo(58, 3);
    await expect(quota).toContainText(/00d02h2[56]m/);
    const reset = quota.getByRole("img", { name: /milliseconds/ });
    const resetRows = await reset
      .locator(":scope > span")
      .evaluateAll((nodes) =>
        nodes.map((node) => {
          const box = node.getBoundingClientRect();
          return { x: box.x, y: box.y, right: box.right, bottom: box.bottom };
        }),
      );
    expect(resetRows).toHaveLength(3);
    for (const [index, row] of resetRows.entries()) {
      expect(row.x).toBeGreaterThan(gaugeBounds.x + gaugeBounds.width);
      expect(row.right).toBeLessThanOrEqual(
        quotaBounds.x + quotaBounds.width - 2,
      );
      if (index) {
        expect(row.x).toBe(resetRows[index - 1].x);
        expect(row.y).toBeGreaterThan(resetRows[index - 1].bottom);
      }
    }
    await expect(
      personal
        .getByRole("region", { name: "Remaining agent usage" })
        .getByRole("img", { name: "Not reported", exact: true }),
    ).toBeVisible();
    await expect(work.getByRole("switch")).toHaveCount(0);
    if (width === 1280) {
      const widthLimit = await layout.evaluate((node) =>
        parseFloat(getComputedStyle(node).maxWidth),
      );
      expect((await work.boundingBox())!.width).toBe(widthLimit);
      expect(usageBounds.x).toBeGreaterThan(
        informationBounds.x + informationBounds.width,
      );
      expect(authBounds.y).toBe(informationBounds.y);
      expect(authBounds.x).toBeGreaterThan(
        informationBounds.x + informationBounds.width,
      );
      expect(usageBounds.width).toBeCloseTo(informationBounds.width, 0);
      expect(informationBounds.width).toBe(424);
      expect(sessionsBounds.y + sessionsBounds.height).toBeCloseTo(
        informationBounds.y + informationBounds.height,
        0,
      );
      expect(usageBounds.y + usageBounds.height).toBeCloseTo(
        sessionsBounds.y + sessionsBounds.height,
        0,
      );
      expect(sessionsBounds.height).toBeLessThan(usageBounds.height);
    } else {
      expect(authBounds.y).toBeGreaterThan(
        informationBounds.y + informationBounds.height,
      );
      expect(usageBounds.width).toBe(informationBounds.width);
    }
    const details = work.getByRole("region", { name: "Account details" });
    await expect(
      details.getByRole("img", { name: "Codex", exact: true }),
    ).toBeVisible();
    const typography = await details
      .locator("dl > div")
      .first()
      .evaluate((el) => ({
        label: parseFloat(getComputedStyle(el.querySelector("dt")!).fontSize),
        value: parseFloat(getComputedStyle(el.querySelector("dd")!).fontSize),
        gap: parseFloat(getComputedStyle(el).rowGap),
      }));
    expect(typography.label).toBeLessThan(typography.value);
    expect(typography.value).toBe(12);
    expect(typography.label).toBe(10);
    expect(typography.gap).toBeGreaterThan(0);
    const fields = await details
      .locator("dl > div")
      .evaluateAll((nodes) =>
        nodes.map((node) => node.getBoundingClientRect().toJSON()),
      );
    for (let index = 0; index < fields.length; index++) {
      if (index % 2) {
        expect(fields[index].x).toBeGreaterThan(fields[index - 1].x);
        expect(fields[index].y).toBe(fields[index - 1].y);
      } else {
        expect(fields[index].x).toBe(fields[0].x);
        if (index >= 2)
          expect(fields[index].y).toBeGreaterThanOrEqual(
            fields[index - 2].y + fields[index - 2].height,
          );
      }
    }
    const metrics = await usage
      .locator("dl > div")
      .evaluateAll((nodes) =>
        nodes.map((node) => node.getBoundingClientRect().toJSON()),
      );
    expect(metrics[1].y).toBe(metrics[0].y);
    expect(metrics[1].x).toBeGreaterThan(metrics[0].x);
    expect(metrics[3].y).toBe(metrics[0].y);
    expect(metrics[4].y).toBeGreaterThan(metrics[0].y);
    const logo = (await details
      .getByRole("img", { name: "Codex", exact: true })
      .boundingBox())!;
    const agent = details
      .locator("dl > div")
      .filter({ has: page.getByText("Agent", { exact: true }) });
    await expect(agent.locator("dd")).toContainText("codex");
    const value = (await agent.locator("dd").boundingBox())!;
    expect(logo.x).toBeGreaterThanOrEqual(value.x);
    expect(logo.y).toBeGreaterThanOrEqual(value.y);
    expect(logo.y + logo.height).toBeLessThanOrEqual(value.y + value.height);
    await expect(work.locator("header")).toHaveCount(0);
    const created = details
      .locator("dl > div")
      .filter({ has: page.getByText("Created", { exact: true }) })
      .locator("dd time");
    await expect(created).toHaveAttribute("datetime", /T.*Z$/);
    await expect(created).toHaveText(
      /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{2}:\d{2}$/,
    );
    await expect(
      details
        .locator("dl > div")
        .filter({ has: page.getByText("Updated", { exact: true }) })
        .locator("dd time"),
    ).toHaveAttribute("datetime", /T.*Z$/);

    await expect(personal).toBeAttached();
    await expect(
      work.getByRole("region", { name: "Sessions using this account" }),
    ).toContainText("Web account layout");
    await expect(work).not.toContainText("Claude project");
    await expect(personal).toContainText("Claude project");
    const personalAuth = personal.getByRole("region", {
      name: "Authentication",
      exact: true,
    });
    await expect(personalAuth).toContainText("Login belongs to each session.");
    await expect(personalAuth.getByRole("button")).toHaveCount(0);
    await expect(personalAuth).not.toContainText("Session-scoped login");
    await expect(
      work.getByRole("button", { name: "Log in", exact: true }),
    ).toHaveCount(0);
    await expect(
      work.getByRole("button", { name: "Log out", exact: true }),
    ).toBeEnabled();
    const authentication = work.getByRole("region", {
      name: "Authentication",
      exact: true,
    });
    const row = [
      authentication.getByRole("button", { name: "Log out", exact: true }),
      authentication.getByText("Signed in", { exact: true }).locator(".."),
    ];
    await expect(
      authentication.getByText("Authentication", { exact: true }),
    ).toHaveCount(0);
    await expect(
      authentication.getByRole("button", {
        name: "Refresh status",
        exact: true,
      }),
    ).toHaveCount(0);
    const boxes = await Promise.all(row.map((item) => item.boundingBox()));
    for (let index = 1; index < boxes.length; index++) {
      if (width < 700) {
        expect(boxes[index]!.y).toBeGreaterThanOrEqual(
          boxes[0]!.y + boxes[0]!.height,
        );
        expect(boxes[index]!.x + boxes[index]!.width / 2).toBeCloseTo(
          boxes[0]!.x + boxes[0]!.width / 2,
          0,
        );
      } else {
        expect(boxes[index]!.x).toBeGreaterThanOrEqual(
          boxes[index - 1]!.x + boxes[index - 1]!.width,
        );
        expect(boxes[index]!.y + boxes[index]!.height / 2).toBeCloseTo(
          boxes[0]!.y + boxes[0]!.height / 2,
          0,
        );
      }
    }
    const main = (await page.getByRole("main").boundingBox())!;
    const card = (await work.boundingBox())!;
    const add = (await page
      .getByRole("button", { name: "Add account", exact: true })
      .boundingBox())!;
    expect(
      Math.abs(card.x + card.width / 2 - main.x - main.width / 2),
    ).toBeLessThan(1);
    expect(add.x + add.width).toBeCloseTo(card.x + card.width, 0);
    expect(add.y + add.height).toBeLessThan(card.y);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`account-columns-${width}.png`),
    });
  });
}

test("account sessions use compact read-only identities and keep selection scoped to the account", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  const work = page.getByRole("article", { name: "Work", exact: true });
  const personal = page.getByRole("article", { name: "Personal", exact: true });
  const table = work.getByRole("table", { name: "Sessions", exact: true });
  const rows = table.locator("tbody tr");
  await expect(rows).toHaveCount(2);
  await expect(table.getByRole("columnheader")).toHaveText([
    "",
    "Name",
    "Project",
    "",
    "",
  ]);
  await expect(
    table.getByRole("columnheader", { name: "Account", exact: true }),
  ).toHaveCount(0);
  await expect(table.locator(".agent-brand")).toHaveCount(0);
  await expect(table.locator(".editable-value-trigger, fieldset")).toHaveCount(
    0,
  );
  await expect(
    rows.first().getByRole("img", { name: "Idle", exact: true }),
  ).toBeVisible();
  await expect(
    rows.nth(1).getByRole("img", { name: "Working", exact: true }),
  ).toBeVisible();
  await expect(
    personal.getByRole("img", { name: "Stopped", exact: true }),
  ).toBeAttached();
  for (const icon of await table.locator("tbody .session-indicator").all()) {
    const bounds = await icon.evaluate((node) => {
      const cell = node.closest("td")!.getBoundingClientRect();
      const glyph = node.getBoundingClientRect();
      return {
        x: glyph.x + glyph.width / 2 - cell.x - cell.width / 2,
        y: glyph.y + glyph.height / 2 - cell.y - cell.height / 2,
        width: cell.width,
      };
    });
    expect(Math.abs(bounds.x)).toBeLessThan(1);
    expect(Math.abs(bounds.y)).toBeLessThan(1);
    expect(bounds.width).toBe(32);
  }
  const name = rows.first().getByText("Web account layout", { exact: true });
  const alias = rows.first().getByText("oak", { exact: true });
  const nameBounds = (await name.boundingBox())!;
  const aliasBounds = (await alias.boundingBox())!;
  expect(aliasBounds.x).toBe(nameBounds.x);
  expect(aliasBounds.y).toBeGreaterThanOrEqual(
    nameBounds.y + nameBounds.height,
  );
  expect(
    await alias.evaluate((node) => parseFloat(getComputedStyle(node).fontSize)),
  ).toBeLessThan(
    await name.evaluate((node) => parseFloat(getComputedStyle(node).fontSize)),
  );
  await expect(
    rows
      .first()
      .getByRole("link", { name: "Web account layout oak", exact: true }),
  ).toHaveAttribute("href", "/c/_/session/oak");
  await expect(
    rows.first().getByRole("link", { name: "Web workspace web", exact: true }),
  ).toHaveAttribute("href", "/c/_/project/web");
  await expect(
    rows.nth(1).getByRole("link", { name: "API workspace api", exact: true }),
  ).toHaveAttribute("href", "/c/_/project/api");
  const projectTitle = rows.first().getByText("Web workspace", { exact: true });
  const projectAlias = rows.first().getByText("web", { exact: true });
  const projectTitleBounds = (await projectTitle.boundingBox())!;
  const projectAliasBounds = (await projectAlias.boundingBox())!;
  expect(projectAliasBounds.x).toBe(projectTitleBounds.x);
  expect(projectAliasBounds.y).toBeGreaterThanOrEqual(
    projectTitleBounds.y + projectTitleBounds.height,
  );
  expect(
    await projectAlias.evaluate((node) =>
      parseFloat(getComputedStyle(node).fontSize),
    ),
  ).toBeLessThan(
    await projectTitle.evaluate((node) =>
      parseFloat(getComputedStyle(node).fontSize),
    ),
  );
  for (const link of await rows.first().getByRole("link").all()) {
    const spacing = await link.evaluate((node) => {
      const cell = node.closest("td")!;
      const style = getComputedStyle(cell);
      const outer = cell.getBoundingClientRect();
      const inner = node.getBoundingClientRect();
      return {
        top: inner.top - outer.top,
        bottom: outer.bottom - inner.bottom,
        paddingTop: style.paddingTop,
        paddingBottom: style.paddingBottom,
      };
    });
    expect(spacing.paddingTop).toBe(spacing.paddingBottom);
    expect(spacing.top).toBeCloseTo(spacing.bottom, 1);
    await expect(link).toHaveCSS("border-top-width", "0px");
    await expect(link).toHaveCSS("box-shadow", "none");
    const background = await link.evaluate(
      (node) => getComputedStyle(node).backgroundColor,
    );
    expect(background).not.toBe("rgba(0, 0, 0, 0)");
    const shades = await link.evaluate((node) => {
      const context = document.createElement("canvas").getContext("2d")!;
      const shade = (element: Element) => {
        context.clearRect(0, 0, 1, 1);
        context.fillStyle = getComputedStyle(element).backgroundColor;
        context.fillRect(0, 0, 1, 1);
        return context.getImageData(0, 0, 1, 1).data[0]!;
      };
      return { button: shade(node), table: shade(node.closest("section")!) };
    });
    expect(shades.button).toBeGreaterThan(shades.table);
    expect(shades.button - shades.table).toBeLessThan(24);
    await link.hover();
    await expect(link).not.toHaveCSS("background-color", background);
  }
  await expect(table.getByRole("textbox")).toHaveCount(0);
  const bulk = table
    .locator("thead")
    .getByRole("button", { name: "Selected session actions", exact: true });
  await bulk.click();
  const bulkMenu = page.getByRole("menu", {
    name: "Selected session actions",
    exact: true,
  });
  await expect(bulkMenu.getByRole("status")).toHaveText("0 selected");
  for (const item of await bulkMenu.getByRole("menuitem").all())
    await expect(item).toBeDisabled();
  await page.keyboard.press("Escape");
  await rows.first().getByRole("checkbox").check();
  await bulk.click();
  await expect(bulkMenu.getByRole("status")).toHaveText("1 selected");
  await bulkMenu.getByRole("menuitem", { name: "Erase", exact: true }).click();
  const erase = page.getByRole("dialog", {
    name: "Erase session",
    exact: true,
  });
  await expect(erase).toContainText("Web account layout");
  await expect(erase).not.toContainText("Review deployment changes");
  await erase.getByRole("button", { name: "Cancel", exact: true }).click();
  await personal
    .getByRole("button", { name: "Selected session actions", exact: true })
    .click();
  await expect(bulkMenu.getByRole("status")).toHaveText("0 selected");
  await page.keyboard.press("Escape");
  const divider = await rows
    .first()
    .locator("td")
    .first()
    .evaluate((node) => getComputedStyle(node).boxShadow);
  expect(divider).not.toBe("none");
  const before = await rows
    .nth(1)
    .evaluate((node) => getComputedStyle(node).backgroundColor);
  await rows.nth(1).hover({ position: { x: 16, y: 20 } });
  await expect(rows.nth(1)).not.toHaveCSS("background-color", before);
  const colors = () =>
    rows
      .first()
      .evaluate((node) =>
        [node, ...node.querySelectorAll("a")].map(
          (element) => getComputedStyle(element).backgroundColor,
        ),
      );
  await page.mouse.move(0, 0);
  await rows.first().evaluate(async (node) => {
    await Promise.all(
      node
        .getAnimations({ subtree: true })
        .filter((animation) => animation instanceof CSSTransition)
        .map((animation) => animation.finished.catch(() => {})),
    );
  });
  const selectedColors = await colors();
  for (const row of [rows.nth(1), rows.first()]) {
    await row.hover({ position: { x: 16, y: 20 } });
    await expect
      .poll(() =>
        row.evaluate((node) => {
          const context = document.createElement("canvas").getContext("2d")!;
          const shade = (element: Element) => {
            context.clearRect(0, 0, 1, 1);
            context.fillStyle = getComputedStyle(element).backgroundColor;
            context.fillRect(0, 0, 1, 1);
            return context.getImageData(0, 0, 1, 1).data[0]!;
          };
          const rowShade = shade(node);
          return [...node.querySelectorAll("a")].every(
            (button) => shade(button) > rowShade,
          );
        }),
      )
      .toBe(true);
  }
  await page.mouse.move(0, 0);
  await expect.poll(colors).toEqual(selectedColors);
  const viewport = work.locator('[data-scroll-area="viewport"]');
  expect(
    await viewport.evaluate((node) => node.scrollWidth <= node.clientWidth),
  ).toBe(true);
  const more = rows.first().getByRole("button", {
    name: "Controls for Web account layout",
    exact: true,
  });
  await more.click();
  await expect(
    page.getByRole("menuitem", { name: "Open conversation", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("menuitem", { name: "Erase", exact: true }),
  ).toHaveCSS(
    "font-weight",
    await page
      .getByRole("menuitem", { name: "Stop", exact: true })
      .evaluate((node) => getComputedStyle(node).fontWeight),
  );
  await page.keyboard.press("Escape");
  await page.screenshot({
    path: testInfo.outputPath("account-session-table.png"),
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});

test("the top add action opens a modal and creates an account card", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  await page.getByRole("button", { name: "Add account", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add account", exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator('[data-dialog-trigger="true"]')).toHaveCount(0);
  await dialog.getByRole("combobox", { name: "Agent", exact: true }).click();
  await dialog.getByRole("option", { name: "Claude", exact: true }).click();
  await expect(
    dialog
      .getByRole("combobox", { name: "Agent", exact: true })
      .locator(".agent-brand-claude img"),
  ).toBeVisible();
  await dialog
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("new-account");
  await dialog
    .getByRole("textbox", { name: "Display name", exact: true })
    .fill("New team account");
  await dialog.getByRole("button", { name: "Add", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  const card = page.getByRole("article", {
    name: "New team account",
    exact: true,
  });
  await expect(card).toBeInViewport();
  await expect(
    card.getByRole("region", { name: "Account details" }),
  ).toContainText("new-account");
  expect(errors).toEqual([]);
});

test("dialogs respect reduced motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  await page.getByRole("button", { name: "Add account", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add account", exact: true });
  await expect(dialog).toBeVisible();
  expect(
    await dialog
      .locator("[data-anchored]")
      .evaluate((node) => getComputedStyle(node).animationName),
  ).toBe("none");
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  const confirmation = page.getByRole("dialog", {
    name: "Log out · Work",
    exact: true,
  });
  expect(
    await confirmation
      .locator("[data-anchored]")
      .evaluate((node) => getComputedStyle(node).animationName),
  ).toBe("none");
});

for (const theme of ["dark", "light"]) {
  test(`account and confirmation dialogs share a translucent blurred surface (${theme})`, async ({
    page,
  }) => {
    await page.goto(
      `/iframe.html?id=resources-views--accounts&viewMode=story&globals=theme:${theme}`,
    );
    await page
      .getByRole("button", { name: "Add account", exact: true })
      .click();
    const addition = page.getByRole("dialog", {
      name: "Add account",
      exact: true,
    });
    const appearance = (node: HTMLElement) => {
      const surface = node.querySelector<HTMLElement>("[data-anchored]")!;
      const style = getComputedStyle(surface);
      const heading = getComputedStyle(node.querySelector(".card-heading")!);
      const body = getComputedStyle(node.querySelector(".card-body")!);
      const footer = getComputedStyle(node.querySelector(".card-footer")!);
      return {
        background: style.backgroundColor,
        blur: style.backdropFilter,
        radius: style.borderRadius,
        backdrop: getComputedStyle(node, "::backdrop").backgroundColor,
        headerPadding: heading.padding,
        bodyPadding: body.padding,
        bodyBackground: body.backgroundColor,
        bodyRadius: body.borderRadius,
        footerPadding: footer.padding,
      };
    };
    const initial = await addition.evaluate(appearance);
    expect(initial.blur).toMatch(/^blur\(/);
    expect(initial.bodyBackground).not.toBe("rgba(0, 0, 0, 0)");
    expect(initial.bodyRadius).not.toBe("0px");
    const alpha = await addition.locator("[data-anchored]").evaluate((node) => {
      const context = document.createElement("canvas").getContext("2d")!;
      context.fillStyle = getComputedStyle(node).backgroundColor;
      context.fillRect(0, 0, 1, 1);
      return context.getImageData(0, 0, 1, 1).data[3];
    });
    expect(alpha).toBeGreaterThan(0);
    expect(alpha).toBeLessThan(255);
    // Blur belongs to the animated surface itself, not a descendant inside its opacity layer.
    await expect(addition.locator(".floating-card")).toHaveCSS(
      "backdrop-filter",
      "none",
    );
    await page.keyboard.press("Escape");
    await page.getByRole("button", { name: "Log out", exact: true }).click();
    const confirmation = page.getByRole("dialog", {
      name: "Log out · Work",
      exact: true,
    });
    expect(await confirmation.evaluate(appearance)).toEqual(initial);
  });
}

test("logout needs confirmation and then enables login", async ({ page }) => {
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  const actionBefore = (await page
    .getByRole("button", { name: "Log out", exact: true })
    .boundingBox())!;
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Log out · Work",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByText("Signed in", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  await dialog.getByRole("button", { name: "Log out", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Log in", exact: true }),
  ).toBeEnabled();
  await expect(
    page.getByRole("button", { name: "Log out", exact: true }),
  ).toHaveCount(0);
  const actionAfter = (await page
    .getByRole("button", { name: "Log in", exact: true })
    .boundingBox())!;
  expect(actionAfter.width).toBe(actionBefore.width);
  expect(actionAfter.x).toBe(actionBefore.x);
});
