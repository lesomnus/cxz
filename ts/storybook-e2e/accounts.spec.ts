import { expect, test } from "@playwright/test";

for (const width of [1280, 390]) {
  test(`account cards are centered without a sidebar at ${width}px`, async ({
    page,
  }) => {
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
    await expect(sections).toHaveCount(4);
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
    await expect(personal).toContainText("Session-scoped login");
    await expect(
      work.getByRole("button", { name: "Log in", exact: true }),
    ).toBeDisabled();
    await expect(
      work.getByRole("button", { name: "Log out", exact: true }),
    ).toBeEnabled();
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
  });
}

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
  ).toBeDisabled();
});
