import { expect, test } from "@playwright/test";

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
    const group = work.locator('[data-section-layout="group"]');
    await expect(group).toHaveCount(1);
    const sections = group.locator(':scope > [data-section-layout="section"]');
    await expect(sections).toHaveCount(3);
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
      usage.getByText("Complete retained coverage", { exact: true }),
    ).toBeVisible();
    const informationBounds = (await group.boundingBox())!;
    const usageBounds = (await usage.boundingBox())!;
    const sessionsBounds = (await sessions.boundingBox())!;
    const authBounds = (await work
      .getByRole("region", { name: "Authentication", exact: true })
      .boundingBox())!;
    expect(usageBounds.x).toBe(informationBounds.x);
    expect(usageBounds.width).toBe(informationBounds.width);
    expect(authBounds.y).toBeGreaterThanOrEqual(
      usageBounds.y + usageBounds.height,
    );
    if (width === 1280) {
      const widthLimit = await layout.evaluate((node) =>
        parseFloat(getComputedStyle(node).maxWidth),
      );
      expect((await work.boundingBox())!.width).toBe(widthLimit);
      expect(sessionsBounds.x).toBeGreaterThan(
        informationBounds.x + informationBounds.width,
      );
      expect(sessionsBounds.y).toBe(informationBounds.y);
      expect(sessionsBounds.width).toBeCloseTo(informationBounds.width, 0);
      expect(informationBounds.width).toBe(420);
      expect(authBounds.y + authBounds.height).toBeCloseTo(
        informationBounds.y + informationBounds.height,
        0,
      );
    } else {
      expect(sessionsBounds.y).toBeGreaterThan(
        informationBounds.y + informationBounds.height,
      );
      expect(sessionsBounds.width).toBe(informationBounds.width);
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
    expect(metrics[2].y).toBeGreaterThan(metrics[0].y);
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
    const created = details.locator("time");
    await expect(created).toHaveAttribute("datetime", /T.*Z$/);
    await expect(created).toHaveText(
      /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{2}:\d{2}$/,
    );

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
      authentication.getByText("Signed in", { exact: true }).locator(".."),
      authentication.getByRole("button", { name: "Log out", exact: true }),
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
      expect(boxes[index]!.x).toBeGreaterThanOrEqual(
        boxes[index - 1]!.x + boxes[index - 1]!.width,
      );
      expect(boxes[index]!.y + boxes[index]!.height / 2).toBeCloseTo(
        boxes[0]!.y + boxes[0]!.height / 2,
        0,
      );
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
    "State",
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
    const shades = await link.evaluate((node) => ({
      button: Number(getComputedStyle(node).backgroundColor.match(/\d+/)?.[0]),
      table: Number(
        getComputedStyle(node.closest("section")!).backgroundColor.match(
          /\d+/,
        )?.[0],
      ),
    }));
    expect(shades.button).toBeGreaterThan(shades.table);
    expect(shades.button - shades.table).toBeLessThan(24);
    await link.hover();
    await expect(link).not.toHaveCSS("background-color", background);
  }
  await expect(table.getByRole("textbox")).toHaveCount(0);
  await rows.first().getByRole("checkbox").check();
  await expect(
    work.getByRole("button", { name: "1 selected", exact: true }),
  ).toBeEnabled();
  await expect(
    personal.getByRole("button", { name: "0 selected", exact: true }),
  ).toBeDisabled();
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
  for (const row of [rows.nth(1), rows.first()]) {
    await row.hover({ position: { x: 16, y: 20 } });
    await expect
      .poll(() =>
        row.evaluate((node) => {
          const rowShade = Number(
            getComputedStyle(node).backgroundColor.match(/\d+/)?.[0],
          );
          return [...node.querySelectorAll("a")].every(
            (button) =>
              Number(
                getComputedStyle(button).backgroundColor.match(/\d+/)?.[0],
              ) > rowShade,
          );
        }),
      )
      .toBe(true);
  }
  await rows
    .first()
    .getByRole("link")
    .evaluateAll((links) =>
      Promise.all(
        links.flatMap((link) =>
          link
            .getAnimations()
            .map((animation) => animation.finished.catch(() => {})),
        ),
      ),
    );
  const selectedColors = await rows
    .first()
    .evaluate((node) =>
      [node, ...node.querySelectorAll("a")].map(
        (element) => getComputedStyle(element).backgroundColor,
      ),
    );
  await page.mouse.move(0, 0);
  await expect
    .poll(() =>
      rows
        .first()
        .evaluate((node) =>
          [node, ...node.querySelectorAll("a")].map(
            (element) => getComputedStyle(element).backgroundColor,
          ),
        ),
    )
    .toEqual(selectedColors);
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
    page.getByRole("menuitem", { name: "Purge", exact: true }),
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
