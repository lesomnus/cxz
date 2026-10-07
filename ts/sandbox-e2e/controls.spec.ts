import { test, expect } from "@playwright/test";
test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});
test("provider dropdowns, quota popovers, aligned headings and bounded press/shadow", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await expect(
    page.locator(".tree-chevron, .tree-branch, .tree-project svg"),
  ).toHaveCount(0);
  await expect(page.locator(".session-title").first()).toHaveText(
    "Project checklist",
  );
  await expect(page.locator(".session-description").first()).toHaveText(
    "session-1sandbox-claude",
  );
  const card = page.locator(".tree-session").first();
  const title = card.locator(".session-title");
  const heading = card.locator(".session-heading");
  const logo = card.getByRole("img", { name: "Claude", exact: true });
  const description = card.locator(".session-description");
  const titleBefore = (await title.boundingBox())!;
  const headingBefore = (await heading.boundingBox())!;
  const logoBefore = (await logo.boundingBox())!;
  const descriptionBefore = (await description.boundingBox())!;
  const indicator = card.locator(".session-indicator");
  const indicatorBefore = (await indicator.boundingBox())!;
  expect(logoBefore.x + logoBefore.width).toBeLessThan(titleBefore.x);
  expect(logoBefore.width).toBe(12);
  expect(logoBefore.height).toBe(12);
  expect(
    logoBefore.y +
      logoBefore.height / 2 -
      titleBefore.y -
      titleBefore.height / 2,
  ).toBeCloseTo(1, 1);
  await card.hover();
  await page.mouse.down();
  const scale = await heading.evaluate((el) =>
    Number(getComputedStyle(el).getPropertyValue("--press-scale")),
  );
  await expect
    .poll(async () => (await title.boundingBox())!.width)
    .toBeCloseTo(titleBefore.width * scale, 1);
  expect(
    titleBefore.width - (await title.boundingBox())!.width,
  ).toBeLessThanOrEqual(4.1);
  const logoPressed = (await logo.boundingBox())!;
  const titlePressed = (await title.boundingBox())!;
  expect(logoPressed.width).toBeLessThan(logoBefore.width);
  expect(logoPressed.width / logoBefore.width).toBeCloseTo(
    titlePressed.width / titleBefore.width,
    3,
  );
  expect(logoPressed.x + logoPressed.width / 2).toBeCloseTo(
    logoBefore.x + logoBefore.width / 2,
    2,
  );
  expect(logoPressed.y + logoPressed.height / 2).toBeCloseTo(
    logoBefore.y + logoBefore.height / 2,
    2,
  );
  expect(titlePressed.x).toBeCloseTo(titleBefore.x, 2);
  expect(await heading.boundingBox()).toEqual(headingBefore);
  expect(await indicator.boundingBox()).toEqual(indicatorBefore);
  expect(await description.boundingBox()).toEqual(descriptionBefore);
  await expect(card.locator(".button-content")).toHaveCSS("transform", "none");
  await page.mouse.move(1000, 20);
  await page.mouse.up();
  await expect
    .poll(async () => (await title.boundingBox())!.width)
    .toBeCloseTo(titleBefore.width, 1);
  const input = page.locator("article.input").first();
  await expect(input.locator(".input-prefix")).toHaveText(">");
  await expect(input).not.toContainText("You");
  const bodyBox = (await input.locator(".message-body").boundingBox())!;
  const prefixBox = (await input.locator(".input-prefix").boundingBox())!;
  expect(bodyBox.y).toBeCloseTo(prefixBox.y, 1);
  expect(bodyBox.x).toBeGreaterThan(prefixBox.x + prefixBox.width);
  const timestamp = input.locator(".input-box .input-time time");
  await expect(timestamp).not.toContainText(/\d{4}/);
  const recorded = await timestamp.getAttribute("datetime");
  expect(Math.abs(Date.now() - Date.parse(recorded!))).toBeLessThan(60_000);
  await expect(input.locator(".input-relative-time")).toHaveText(
    "Less than a minute ago",
  );
  const timeBox = (await timestamp.boundingBox())!;
  const inputBox = (await input.locator(".input-box").boundingBox())!;
  expect(timeBox.y).toBeGreaterThan(inputBox.y);
  expect(timeBox.y + timeBox.height).toBeLessThan(prefixBox.y);
  expect(timeBox.x).toBeCloseTo(prefixBox.x, 1);
  expect(timeBox.x + timeBox.width).toBeLessThan(inputBox.x + inputBox.width);
  const userHeading = await page
    .locator("article.input .input-prefix")
    .first()
    .boundingBox();
  const agentHeading = await page
    .locator("article:not(.input) > small")
    .first()
    .boundingBox();
  expect(userHeading!.x).toBe(agentHeading!.x);
  expect(
    await page
      .locator(".markdown")
      .first()
      .evaluate((el) => getComputedStyle(el).paddingLeft),
  ).toBe("12px");
  await expect(page.getByText("Actions", { exact: true })).toHaveCount(0);
  await page.locator(".quota-dots").hover();
  await expect(page.getByRole("tooltip")).toContainText("Reset");
  await expect(page.getByRole("tooltip")).toContainText("70% remaining");
  await page.locator(".context-popover").hover();
  await expect(
    page.getByRole("tooltip", { name: "24K/200K", exact: true }),
  ).toBeVisible();
  const effortX = (await page.locator(".effort-field").boundingBox())!.x;
  const effort = page.getByRole("combobox", { name: "Effort", exact: true });
  const model = page.getByRole("combobox", { name: "Model", exact: true });
  await effort.click();
  await page.getByRole("option", { name: "low", exact: true }).click();
  await expect(page.locator(".effort-field .meta-value")).toHaveText("low");
  // Labels stay inert; opening the value overlays its exact pre-open text origin.
  await page.locator(".model-field .meta-label").click();
  await expect(page.getByRole("listbox")).toHaveCount(0);
  const currentBefore = (await model.locator(".meta-value").boundingBox())!;
  const baseline = (await page
    .locator(".model-field .meta-label")
    .boundingBox())!;
  expect(
    Math.abs(
      currentBefore.y + currentBefore.height - baseline.y - baseline.height,
    ),
  ).toBeLessThan(0.1);
  await expect(model).toHaveCSS("padding", "2px 6px");
  expect(currentBefore.x - (await model.boundingBox())!.x).toBe(6);
  await expect(page.locator("form.composer")).toHaveCSS("padding-top", "0px");
  await model.click();
  const choices = page.getByRole("listbox", { name: "Model choices" });
  await expect(choices).toBeVisible();
  await expect(choices).toHaveCSS("padding", "2px");
  const currentAfter = (await choices
    .locator(".setting-current .meta-value")
    .boundingBox())!;
  expect(Math.abs(currentAfter.x - currentBefore.x)).toBeLessThan(0.1);
  expect(Math.abs(currentAfter.y - currentBefore.y)).toBeLessThan(0.1);
  expect(Math.abs(currentAfter.width - currentBefore.width)).toBeLessThan(0.1);
  const menuBox = (await choices.boundingBox())!;
  const labelBox = (await page
    .locator(".model-field .meta-label")
    .boundingBox())!;
  expect(menuBox.x).toBeGreaterThanOrEqual(labelBox.x + labelBox.width);
  expect(menuBox.y).toBeGreaterThanOrEqual(0);
  expect(menuBox.y + menuBox.height).toBeLessThanOrEqual(1000);
  const selectedRow = choices.getByRole("option", { selected: true });
  const otherRow = choices.getByRole("option", { selected: false }).first();
  await otherRow.hover();
  await expect(otherRow).toHaveCSS("color", "rgb(255, 255, 255)");
  expect((await selectedRow.boundingBox())!.width).toBe(
    (await otherRow.boundingBox())!.width,
  );
  expect(await selectedRow.evaluate((el) => getComputedStyle(el).padding)).toBe(
    await otherRow.evaluate((el) => getComputedStyle(el).padding),
  );
  await expect(choices.getByRole("separator")).toBeVisible();
  await page.screenshot({
    path: "test-results/sandbox-value-menu.png",
    fullPage: true,
  });
  await page
    .getByRole("option", { name: "sandbox-claude-compact", exact: true })
    .click();
  await expect(page.locator(".model-field .meta-value")).toHaveText(
    "sandbox-claude-compact",
  );
  await expect(page.locator(".effort-field .meta-value")).toHaveText("high");
  await effort.press("ArrowUp");
  await expect(
    page.getByRole("listbox", { name: "Effort choices" }),
  ).toBeVisible();
  await expect(
    page.getByRole("option", { name: "medium", exact: true }),
  ).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("listbox")).toHaveCount(0);
  await expect(effort).toBeFocused();
  const quota = page.locator(".quota-dots");
  expect(await quota.evaluate((el) => getComputedStyle(el).fontSize)).toBe(
    await page
      .locator(".model-field .meta-label")
      .evaluate((el) => getComputedStyle(el).fontSize),
  );
  const quotaBounds = (await quota.boundingBox())!,
    donutBounds = (await page.locator(".context-donut").boundingBox())!;
  expect(
    donutBounds.x - quotaBounds.x - quotaBounds.width,
  ).toBeGreaterThanOrEqual(7);
  expect((await page.locator(".effort-field").boundingBox())!.x).toBe(effortX);
  await expect(page.locator("article.input")).toHaveCount(1);
  await expect(page.locator(".conversation header small")).toContainText(
    "idle",
  );
  // Every button scales uniformly; a long label loses at most 4px along its long edge.
  const project = page.locator(".tree-project").first();
  const original = (await project.locator(".button-content").boundingBox())!;
  await project.hover();
  await page.mouse.down();
  await expect
    .poll(
      async () =>
        (await project.locator(".button-content").boundingBox())!.width,
    )
    .toBeLessThan(original.width - 3.5);
  const pressed = (await project.locator(".button-content").boundingBox())!;
  expect(original.width - pressed.width).toBeLessThanOrEqual(4.1);
  expect(pressed.width / original.width).toBeCloseTo(
    pressed.height / original.height,
    3,
  );
  await page.mouse.move(1000, 20);
  await page.mouse.up();
  const message = page.getByRole("textbox", { name: "Message", exact: true });
  await message.fill("Hover test");
  const send = page.getByRole("button", { name: "Send", exact: true });
  await send.hover();
  await expect(send).toHaveCSS("background-color", "rgb(54, 54, 54)");
  await page.mouse.move(1000, 20);
  await expect(send).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-3");
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
  await expect(page.locator(".model-field .meta-value")).toHaveText(
    "sandbox-claude",
  );
  // Catalog lookup also works when its event is older than the cached history window.
  await expect(model).toBeEnabled();
  const pane = page.locator(".transcript");
  const shadow = () =>
    page.locator(".transcript-fade-bottom").evaluate((el) => ({
      length: parseFloat(getComputedStyle(el).height),
      opacity: parseFloat(getComputedStyle(el).opacity),
      height: document.querySelector(".composer-input")!.getBoundingClientRect()
        .height,
      background: getComputedStyle(el).backgroundImage,
    }));
  expect((await shadow()).length).toBe(0);
  expect((await shadow()).background).toContain("rgb(20, 20, 20)");
  await pane.evaluate((el) => (el.scrollTop -= 30));
  await expect.poll(async () => (await shadow()).length).toBeGreaterThan(1);
  const shallow = await shadow();
  expect(shallow.opacity).toBeGreaterThan(0);
  expect(shallow.opacity).toBeLessThan(0.2);
  await pane.evaluate((el) => (el.scrollTop -= 200));
  await expect
    .poll(async () => (await shadow()).length)
    .toBeGreaterThan(shallow.length);
  await expect
    .poll(async () => {
      const v = await shadow();
      return Math.abs(v.length - v.height);
    })
    .toBeLessThan(1);
  const layers = await page
    .locator(".transcript-fade-bottom")
    .evaluate((fade) => {
      const bounds = fade.getBoundingClientRect();
      const within = (node: HTMLElement) => {
        const box = node.getBoundingClientRect();
        return (
          box.top + box.height / 2 > bounds.top &&
          box.top + box.height / 2 < bounds.bottom
        );
      };
      const body = [
        ...document.querySelectorAll<HTMLElement>(".markdown > p"),
      ].find(within)!;
      const control = [
        ...document.querySelectorAll<HTMLElement>(
          ".transcript small, .transcript .copy",
        ),
      ].find(within)!;
      const hit = (node: HTMLElement) => {
        const box = node.getBoundingClientRect();
        return document.elementFromPoint(
          box.left + 3,
          box.top + box.height / 2,
        );
      };
      const previous = fade.style.pointerEvents;
      fade.style.pointerEvents = "auto";
      const bodyBelow = hit(body) === fade;
      const controlAbove = control.contains(hit(control));
      fade.style.pointerEvents = previous;
      return { bodyBelow, controlAbove };
    });
  expect(layers).toEqual({ bodyBelow: true, controlAbove: true });
  const latest = page.getByRole("button", { name: "Latest", exact: true });
  // Wait for the overlay's entrance transition before testing its hit bounds.
  await expect
    .poll(() =>
      latest.evaluate((el) => {
        const box = el.getBoundingClientRect();
        const area = el.closest(".composer-toolbar")!.getBoundingClientRect();
        return (
          box.bottom <= area.bottom &&
          document
            .elementFromPoint(
              box.left + box.width / 2,
              box.top + box.height / 2,
            )
            ?.closest("button") === el
        );
      }),
    )
    .toBe(true);
  await page.screenshot({
    path: "test-results/sandbox-input-shadow.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Latest", exact: true }).click();
  await expect.poll(async () => (await shadow()).opacity).toBe(0);
});
