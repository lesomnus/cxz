import { expect, test } from "@playwright/test";

test("confirmation dialog traps focus, cancels without acting and fits a small viewport", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-confirmationdialog--default&viewMode=story",
  );
  const launch = page.getByRole("button", {
    name: "Open confirmation",
    exact: true,
  });
  await launch.click();
  const dialog = page.getByRole("dialog", { name: "Stop session" });
  const cancel = dialog.getByRole("button", { name: "Cancel", exact: true });
  await expect(cancel).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(
    dialog.getByRole("button", { name: "Stop", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(cancel).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(launch).toBeFocused();
  await expect(page.getByRole("status")).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 600 });
  await launch.click();
  const bounds = (await dialog.boundingBox())!;
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  expect(bounds.y).toBeGreaterThanOrEqual(0);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(600);
  await page.screenshot({
    path: "test-results/confirmation-dialog-mobile.png",
  });
  await cancel.click();
  await launch.click();
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("status")).toHaveText("Action completed");
});

test("pending dialogs prevent dismissal and failed actions stay open for retry", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-confirmationdialog--pending&viewMode=story",
  );
  await page
    .getByRole("button", { name: "Open confirmation", exact: true })
    .click();
  let dialog = page.getByRole("dialog", { name: "Stop session" });
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog).toHaveAttribute("aria-busy", "true");
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeDisabled();
  await expect(
    dialog.getByRole("button", { name: "Working…", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeVisible();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("status")).toHaveText("Action completed");
  await page.goto(
    "/iframe.html?id=components-confirmationdialog--failure&viewMode=story",
  );
  await page
    .getByRole("button", { name: "Open confirmation", exact: true })
    .click();
  dialog = page.getByRole("dialog", { name: "Stop session" });
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog.getByRole("alert")).toHaveText(
    "Action failed. Please try again.",
  );
  await expect(
    dialog.getByRole("button", { name: "Stop", exact: true }),
  ).toBeEnabled();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});

test("async free-text questions submit structured native keys and only explicit actions dismiss them", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--free-text&viewMode=story",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  await expect(question).toBeVisible();
  await expect(question).not.toContainText("not supported");
  await expect(question.getByRole("radio")).toHaveCount(0);
  const submit = question.getByRole("button", {
    name: "Submit",
    exact: true,
  });
  await expect(submit).toBeDisabled();
  const answer = question.getByRole("textbox", {
    name: "Answer: Describe the change you want to make.",
    exact: true,
  });
  await answer.fill("first line\nsecond line");
  await expect(question.locator(".editor-gutter > div > div")).toHaveText([
    "1",
    "2",
  ]);
  await page.keyboard.press("Escape");
  await expect(question).toBeVisible();
  await submit.click();
  await expect(question).toHaveCount(0);
  expect(JSON.parse((await page.getByRole("status").innerText())!)).toEqual({
    "0": { selected: [], other: "first line\nsecond line" },
  });
  await page.getByRole("button", { name: "Ask again", exact: true }).click();
  const cancel = question.getByRole("button", { name: "Cancel", exact: true });
  const confirm = question.getByRole("button", {
    name: "Confirm cancel",
    exact: true,
  });
  await cancel.hover();
  await expect(cancel).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(cancel).toHaveCSS("border-top-width", "1px");
  const cancelBounds = await cancel.boundingBox();
  await cancel.click();
  await expect(question).toBeVisible();
  await expect(confirm).toHaveAttribute("data-confirming", "true");
  await expect(confirm).toHaveCSS("background-color", "rgb(59, 27, 27)");
  expect(await confirm.boundingBox()).toEqual(cancelBounds);
  await page.screenshot({
    path: "test-results/question-cancel-confirmation.png",
  });
  await answer.hover();
  await expect(cancel).toHaveAttribute("data-confirming", "false");
  await cancel.focus();
  await page.keyboard.press("Enter");
  await expect(confirm).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(cancel).toHaveAttribute("data-confirming", "false");
  await cancel.focus();
  await page.keyboard.press("Enter");
  await page.keyboard.press("Escape");
  await expect(cancel).toHaveAttribute("data-confirming", "false");
  await expect(question).toBeVisible();
  await page.keyboard.press("Enter");
  await page.keyboard.press("Space");
  await expect(page.getByRole("status")).toHaveText("Denied");
});

test("question tabs preserve choices, draft and undo and submit all steps together", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--steps&viewMode=story",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  const tabs = question.getByRole("tablist", { name: "Question steps" });
  const submit = question.getByRole("button", {
    name: "Submit",
    exact: true,
  });
  await expect(tabs.getByRole("tab")).toHaveText(["Q1○", "Q2○", "Q3○"]);
  const tabBefore = await tabs
    .getByRole("tab", { name: "Q1", exact: true })
    .boundingBox();
  const iconBefore = await tabs
    .getByRole("tab", { name: "Q1", exact: true })
    .locator(".tab-answered")
    .boundingBox();
  await question.getByRole("radio", { name: "Compact", exact: true }).check();
  expect(
    await tabs.getByRole("tab", { name: "Q1", exact: true }).boundingBox(),
  ).toEqual(tabBefore);
  expect(
    await tabs
      .getByRole("tab", { name: "Q1", exact: true })
      .locator(".tab-answered")
      .boundingBox(),
  ).toEqual(iconBefore);
  await tabs.getByRole("tab", { name: "Q2", exact: true }).click();
  const answer = question.getByRole("textbox", {
    name: "Answer: Describe any additional changes.",
    exact: true,
  });
  const editor = question.locator(
    '.question-group[data-active="true"] .question-other-editor',
  );
  const editorBefore = await editor.boundingBox();
  await answer.fill("Keep this draft");
  await expect(editor).toHaveCSS("outline-width", "1px");
  expect(await editor.boundingBox()).toEqual(editorBefore);
  await answer.press("End");
  await answer.press("!");
  await tabs.getByRole("tab", { name: "Q3", exact: true }).click();
  await expect(submit).toBeDisabled();
  await question.getByRole("radio", { name: "Now", exact: true }).check();
  await tabs.getByRole("tab", { name: "Q2", exact: true }).click();
  await expect(answer).toHaveValue("Keep this draft!");
  await answer.press("Control+z");
  await expect(answer).toHaveValue("Keep this draft");
  await tabs.getByRole("tab", { name: "Q1", exact: true }).focus();
  await page.keyboard.press("End");
  await expect(
    tabs.getByRole("tab", { name: "Q3", exact: true }),
  ).toBeFocused();
  await expect(
    question.getByRole("radio", { name: "Now", exact: true }),
  ).toBeChecked();
  await page.keyboard.press("Home");
  await expect(
    question.getByRole("radio", { name: "Compact", exact: true }),
  ).toBeChecked();
  await page.screenshot({ path: "test-results/question-tabs.png" });
  await submit.click();
  expect(JSON.parse(await page.getByRole("status").innerText())).toEqual({
    layout: { selected: ["Compact"], other: "" },
    notes: { selected: [], other: "Keep this draft" },
    timing: { selected: ["Now"], other: "" },
  });
});

test("async choice and free-text steps use positional keys and option previews render Markdown and code", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--async-steps&viewMode=story",
  );
  let question = page.getByRole("region", { name: "Question", exact: true });
  await question.getByRole("radio", { name: "Dark", exact: true }).check();
  await question.getByRole("tab", { name: "Q1", exact: true }).focus();
  await page.keyboard.press("ArrowRight");
  await expect(
    question.getByRole("tab", { name: "Q2", exact: true }),
  ).toBeFocused();
  await question
    .getByRole("textbox", {
      name: "Answer: What else should we change?",
      exact: true,
    })
    .fill("More contrast");
  await question.getByRole("button", { name: "Submit", exact: true }).click();
  expect(JSON.parse(await page.getByRole("status").innerText())).toEqual({
    "0": { selected: ["Dark"], other: "" },
    "1": { selected: [], other: "More contrast" },
  });
  await page.goto(
    "/iframe.html?id=conversation-questioncard--option-previews&viewMode=story",
  );
  question = page.getByRole("region", { name: "Question", exact: true });
  await expect(
    question.getByRole("heading", { name: "Compact editor", exact: true }),
  ).toBeVisible();
  await expect(
    question.locator(".question-option-preview pre").first(),
  ).toContainText("max-width: 600px");
  const link = question.getByRole("link", { name: "Documentation" });
  await expect(link).toHaveAttribute("target", "_blank");
  await expect(link).toHaveAttribute("rel", "noopener noreferrer");
  await question.getByRole("radio", { name: /Compact/ }).check();
  await page.screenshot({ path: "test-results/question-option-previews.png" });
  await question.getByRole("button", { name: "Submit", exact: true }).click();
  expect(JSON.parse(await page.getByRole("status").innerText())).toEqual({
    "Which implementation should we use?": { selected: ["Compact"], other: "" },
  });
});

test("question layout keeps the longest height, fixed Other/footer, and option-only scrolling", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--long-questions&viewMode=story",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  const tab = (name: string) =>
    question.getByRole("tab", { name, exact: true });
  const footer = question.locator(".question-footer");
  const submit = footer.getByRole("button", { name: "Submit", exact: true });
  const next = footer.getByRole("button", { name: "Next", exact: true });
  await expect(question.locator(".card-heading")).toHaveCount(0);
  await expect(
    question.getByRole("button", { name: "Request details" }),
  ).toHaveCount(0);
  await expect(tab("Q1")).toHaveAttribute("aria-selected", "true");
  await expect(submit).toBeDisabled();
  const initial = (await question.boundingBox())!;
  const conversation = (await page.locator(".conversation").boundingBox())!;
  expect(initial.height).toBeLessThanOrEqual(conversation.height / 2 + 1);
  await question.getByRole("radio", { name: "Compact", exact: true }).check();
  await expect(tab("Q1").locator(".tab-answered")).toHaveAttribute(
    "data-answered",
    "true",
  );
  await next.click();
  await expect(tab("Q2")).toHaveAttribute("aria-selected", "true");
  expect((await question.boundingBox())!.height).toBeCloseTo(initial.height, 0);
  const options = question.locator(
    '.question-group[data-active="true"] .question-options',
  );
  expect(
    await options.evaluate((el) => el.scrollHeight > el.clientHeight),
  ).toBe(true);
  const fixed = await footer.boundingBox();
  const other = question.locator(
    '.question-group[data-active="true"] .question-other',
  );
  const otherBefore = await other.boundingBox();
  const active = question.locator('.question-group[data-active="true"]');
  const topFade = active.locator(".scroll-edge-fade-top");
  const bottomFade = active.locator(".scroll-edge-fade-bottom");
  await expect(topFade).toHaveCSS("opacity", "0");
  await expect(bottomFade).toHaveCSS("opacity", "1");
  const bodyColor = await question
    .locator(".question-body")
    .evaluate((el) => getComputedStyle(el).backgroundColor);
  expect(
    await bottomFade.evaluate((el) => getComputedStyle(el).backgroundImage),
  ).toContain(bodyColor);
  await options.evaluate(
    (el) => (el.scrollTop = (el.scrollHeight - el.clientHeight) / 2),
  );
  await expect(topFade).toHaveCSS("opacity", "1");
  await expect(bottomFade).toHaveCSS("opacity", "1");
  expect((await topFade.boundingBox())!.height).toBeGreaterThan(0);
  expect((await bottomFade.boundingBox())!.height).toBeGreaterThan(0);
  await page.screenshot({
    path: "test-results/question-choice-scroll-fades.png",
  });
  await options.evaluate((el) => (el.scrollTop = el.scrollHeight));
  await expect(topFade).toHaveCSS("opacity", "1");
  await expect(bottomFade).toHaveCSS("opacity", "0");
  expect(await footer.boundingBox()).toEqual(fixed);
  expect(await other.boundingBox()).toEqual(otherBefore);
  await question.getByRole("radio", { name: /^Implementation 12 / }).check();
  await expect(
    question.locator('.question-option-card[data-selected="true"]').last(),
  ).toHaveCSS("outline-width", "1px");
  await next.click();
  await expect(tab("Q3")).toHaveAttribute("aria-selected", "true");
  await expect(next).toBeDisabled();
  await expect(submit).toBeDisabled();
  await question
    .getByRole("textbox", { name: "Answer: Anything else?", exact: true })
    .fill("Keep the footer visible");
  await expect(submit).toBeEnabled();
  expect((await question.boundingBox())!.height).toBeCloseTo(initial.height, 0);
  await tab("Q1").click();
  const compactFade = question.locator(
    '.question-group[data-active="true"] .scroll-edge-fade',
  );
  await expect(compactFade).toHaveCount(2);
  for (const fade of await compactFade.all())
    await expect(fade).toHaveCSS("opacity", "0");
  await expect(
    question.getByRole("radio", { name: "Compact", exact: true }),
  ).toBeChecked();
  await page.screenshot({ path: "test-results/question-long-layout.png" });
  await page.setViewportSize({ width: 390, height: 620 });
  await expect
    .poll(async () => (await question.boundingBox())!.height)
    .toBeLessThanOrEqual(
      (await page.locator(".conversation").boundingBox())!.height / 2 + 1,
    );
  await tab("Q2").click();
  await expect(next).toBeVisible();
  await expect(submit).toBeVisible();
  await expect(other).toBeVisible();
  await page.screenshot({ path: "test-results/question-long-mobile.png" });
});

test("choice fades track remaining distance and disappear on the boundary frame", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--long-questions&viewMode=story",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  await question.getByRole("tab", { name: "Q2", exact: true }).click();
  const samples = await question
    .locator('.question-group[data-active="true"]')
    .evaluate(async (group) => {
      const pane = group.querySelector<HTMLElement>(".question-options")!;
      const top = group.querySelector<HTMLElement>(".scroll-edge-fade-top")!;
      const bottom = group.querySelector<HTMLElement>(
        ".scroll-edge-fade-bottom",
      )!;
      const depth = parseFloat(
        getComputedStyle(group).getPropertyValue("--scroll-fade-depth"),
      );
      const max = pane.scrollHeight - pane.clientHeight;
      const sample = async (position: number, fade: HTMLElement) => {
        pane.scrollTop = position;
        await new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        );
        const style = getComputedStyle(fade);
        return {
          opacity: Number(style.opacity),
          height: parseFloat(style.height),
        };
      };
      const bottomNear = await sample(max - depth / 4, bottom);
      const bottomEnd = await sample(max, bottom);
      const topNear = await sample(depth / 4, top);
      const topEnd = await sample(0, top);
      return { depth, bottomNear, bottomEnd, topNear, topEnd };
    });
  for (const near of [samples.bottomNear, samples.topNear]) {
    expect(near.opacity).toBeGreaterThan(0);
    expect(near.opacity).toBeLessThan(0.5);
    expect(near.height).toBeLessThanOrEqual(samples.depth / 4 + 1);
  }
  for (const end of [samples.bottomEnd, samples.topEnd]) {
    expect(end.opacity).toBe(0);
    expect(end.height).toBe(0);
  }
});

test("questions expand to the available conversation height and collapse from either margin without losing answers", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--long-questions&viewMode=story",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  const original = (await question.boundingBox())!;
  const other = question.getByRole("textbox", {
    name: "Other answer: Choose a layout.",
    exact: true,
  });
  await other.fill("Preserve this draft");
  await question.getByRole("button", { name: "Expand question" }).click();
  const collapse = question.getByRole("button", { name: "Collapse question" });
  await expect(collapse).toHaveAttribute("aria-expanded", "true");
  const expanded = (await question.boundingBox())!;
  const area = (await page.locator(".transcript-area").boundingBox())!;
  const composer = (await page.locator(".composer-wrapper").boundingBox())!;
  expect(expanded.height).toBeGreaterThan(original.height);
  expect(expanded.y).toBeGreaterThanOrEqual(area.y);
  expect(expanded.y - area.y).toBeLessThan(16);
  expect(composer.y - expanded.y - expanded.height).toBeGreaterThanOrEqual(0);
  expect(composer.y - expanded.y - expanded.height).toBeLessThan(20);
  await question.getByText("Choose a layout.", { exact: true }).click();
  await expect(collapse).toBeVisible();
  await page.mouse.move(expanded.x - 10, expanded.y + 30);
  await page.mouse.wheel(0, -250);
  await expect
    .poll(async () => (await question.boundingBox())!.y)
    .toBeCloseTo(expanded.y, 0);
  await page.screenshot({ path: "test-results/question-expanded-desktop.png" });
  await page.mouse.click(expanded.x - 10, expanded.y + 30);
  await expect(
    question.getByRole("button", { name: "Expand question" }),
  ).toBeVisible();
  expect((await question.boundingBox())!.height).toBeCloseTo(
    original.height,
    0,
  );
  await expect(other).toHaveValue("Preserve this draft");
  await question.getByRole("button", { name: "Expand question" }).click();
  const right = (await question.boundingBox())!;
  await page.mouse.click(right.x + right.width + 10, right.y + 30);
  await expect(
    question.getByRole("button", { name: "Expand question" }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 620 });
  await question.getByRole("button", { name: "Expand question" }).focus();
  await page.keyboard.press("Enter");
  const mobile = (await question.boundingBox())!;
  const mobileArea = (await page.locator(".transcript-area").boundingBox())!;
  const mobileComposer = (await page
    .locator(".composer-wrapper")
    .boundingBox())!;
  expect(mobile.y).toBeGreaterThanOrEqual(mobileArea.y);
  expect(mobile.y - mobileArea.y).toBeLessThan(16);
  expect(mobile.y + mobile.height).toBeLessThanOrEqual(mobileComposer.y);
  await expect(
    question.getByRole("button", { name: "Submit", exact: true }),
  ).toBeVisible();
  await page.screenshot({ path: "test-results/question-expanded-mobile.png" });
  await collapse.click();
  await expect(other).toHaveValue("Preserve this draft");
  await expect(question).toBeVisible();
  await page.screenshot({
    path: "test-results/question-expanded-mobile-collapsed.png",
  });
});

test("reading tucks the question into the composer fade and approaching restores it without losing answers", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--long-questions&viewMode=story",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  const layer = page.locator(".question-cards");
  const host = page.locator(".floating-card-host");
  await question.getByRole("radio", { name: "Compact", exact: true }).check();
  const original = (await question.boundingBox())!;
  const area = (await page.locator(".transcript-area").boundingBox())!;
  await page.locator(".transcript").focus();
  await page.mouse.move(area.x + 20, area.y + 50);
  await page.mouse.wheel(0, -240);
  await expect(layer).toHaveAttribute("data-retreated", "true");
  await expect
    .poll(async () => (await question.boundingBox())!.y - original.y)
    .toBeCloseTo(original.height / 2, 0);
  await expect(host.locator(".question-scroll-fade")).toHaveCSS("opacity", "1");
  await page.screenshot({ path: "test-results/question-retreated.png" });
  const retreated = (await question.boundingBox())!;
  await page.mouse.move(retreated.x + retreated.width / 2, retreated.y - 12);
  await expect(layer).toHaveAttribute("data-retreated", "false");
  await expect
    .poll(async () => (await question.boundingBox())!.y)
    .toBeCloseTo(original.y, 0);
  await expect(host.locator(".question-scroll-fade")).toHaveCSS("opacity", "0");
  await expect(
    question.getByRole("radio", { name: "Compact", exact: true }),
  ).toBeChecked();
  await page.keyboard.press("Escape");
  await expect(question).toBeVisible();
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(layer).toHaveCSS("transition-duration", "0s");
});
