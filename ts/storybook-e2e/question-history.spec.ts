import { expect, test, type Locator } from "@playwright/test";

const history = "article.response.question-history";

function content(card: Locator, name: "Question" | "Answer") {
  return card.getByRole("group", { name, exact: true });
}

const slideX = (slide: Locator) =>
  slide.evaluate(
    (node) => new DOMMatrixReadOnly(getComputedStyle(node).transform).m41,
  );

test("recorded questions share response chrome and show each answer inline", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-eventcards--answered-question&viewMode=story",
  );
  const card = page.locator(history);
  await expect(card).toHaveAttribute("data-state", "completed");
  await expect(card.locator(".response-heading .agent-brand")).toBeVisible();
  await expect(card.locator(".response-heading time")).toBeVisible();
  await expect(card.getByText("Question", { exact: true })).toHaveCount(0);
  await expect(card.getByText("Answer", { exact: true })).toHaveCount(0);
  const first = card.getByRole("tab", { name: "Q1", exact: true });
  const notes = card.getByRole("tab", { name: "Q2", exact: true });
  const last = card.getByRole("tab", { name: "Q3", exact: true });
  await expect(first).toHaveAttribute("aria-selected", "true");
  await expect(content(card, "Question")).toContainText("Choose the layout.");
  await expect(content(card, "Answer")).toContainText("Compact");
  await notes.click();
  await expect(content(card, "Question")).toContainText(
    "Describe any additional changes.",
  );
  await expect(content(card, "Answer").locator("pre code")).toHaveText(
    "const compact = true;\n",
  );
  await expect(content(card, "Answer")).not.toContainText("Now");
  await notes.press("ArrowRight");
  await expect(last).toBeFocused();
  await expect(card.getByRole("tabpanel")).toHaveAttribute(
    "aria-labelledby",
    (await last.getAttribute("id"))!,
  );
  await expect(content(card, "Answer")).toContainText("Now");
  await last.press("Home");
  await expect(first).toBeFocused();
  await first.press("End");
  await expect(last).toBeFocused();
  await last.press("ArrowLeft");
  await expect(notes).toBeFocused();
  await card.dblclick({ position: { x: 10, y: 10 } });
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Submit", exact: true }),
  ).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("canceled questions preserve their text and cancellation state in every tab", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-eventcards--canceled-question&viewMode=story",
  );
  const card = page.locator(history);
  await expect(card).toHaveAttribute("data-state", "failed");
  await expect(content(card, "Question")).toContainText("Choose the layout.");
  await expect(content(card, "Answer")).toContainText("Canceled");
  await card.getByRole("tab", { name: "Q2", exact: true }).click();
  await expect(content(card, "Question")).toContainText(
    "Describe any additional changes.",
  );
  await expect(content(card, "Answer")).toContainText("Canceled");
});

test("Question dialog keeps status tabs while history shows numbered surface buttons", async ({
  page,
}) => {
  const appearance = (tab: Locator) =>
    tab.evaluate((node) => {
      const style = getComputedStyle(node);
      const label = node.querySelector("[data-tab-label]")!;
      return {
        width: style.width,
        height: style.height,
        padding: style.padding,
        font: style.font,
        label: label.textContent,
      };
    });
  await page.goto(
    "/iframe.html?id=conversation-questioncard--steps&viewMode=story",
  );
  const live = page.getByRole("region", { name: "Question", exact: true });
  const liveTab = live.getByRole("tab", { name: "Q1", exact: true });
  await expect(liveTab.locator(".tab-answered circle")).toHaveCount(1);
  await expect(liveTab.locator(".tab-answered path")).toHaveCount(0);
  await live.getByRole("radio", { name: "Compact", exact: true }).check();
  await expect(liveTab.locator(".tab-answered path")).toHaveCount(1);
  await page.evaluate(() => document.fonts.ready);
  await expect(liveTab).not.toHaveCSS("border-radius", "0px");
  await expect(liveTab).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(liveTab).toHaveCSS("border-bottom-style", "none");
  await expect(liveTab).toHaveAttribute("data-effect", "sparkle");
  const otherLiveTab = live.getByRole("tab", { name: "Q2", exact: true });
  await otherLiveTab.hover();
  await expect(otherLiveTab).not.toHaveCSS(
    "background-color",
    "rgba(0, 0, 0, 0)",
  );
  const expected = await appearance(liveTab);
  await page.goto(
    "/iframe.html?id=conversation-eventcards--answered-question&viewMode=story",
  );
  const recordedTab = page
    .locator(history)
    .getByRole("tab", { name: "Q1", exact: true });
  await expect(recordedTab.locator('[data-mode="integer"]')).toHaveText("01");
  await expect(page.locator(history).locator(".tab-answered")).toHaveCount(0);
  expect(
    await recordedTab.evaluate((node) => {
      const tab = node.getBoundingClientRect();
      const number = node
        .querySelector('[data-mode="integer"]')!
        .getBoundingClientRect();
      return Math.abs(number.x + number.width / 2 - tab.x - tab.width / 2);
    }),
  ).toBeLessThan(0.5);
  await expect.poll(() => appearance(recordedTab)).toEqual(expected);
  await expect(recordedTab).not.toHaveCSS("border-radius", "0px");
  await expect(recordedTab).toHaveCSS("border-bottom-style", "none");
  await expect(recordedTab).toHaveAttribute("data-effect", "sparkle");
  const questionBackground = await content(
    page.locator(history),
    "Question",
  ).evaluate((node) => getComputedStyle(node).backgroundColor);
  const answerBackground = await content(
    page.locator(history),
    "Answer",
  ).evaluate((node) => getComputedStyle(node).backgroundColor);
  await page.mouse.move(0, 0);
  await expect(recordedTab).toHaveCSS("background-color", answerBackground);
  await recordedTab.hover();
  await expect(recordedTab).toHaveCSS("background-color", questionBackground);
  const otherRecordedTab = page
    .locator(history)
    .getByRole("tab", { name: "Q2", exact: true });
  await otherRecordedTab.hover();
  await expect(otherRecordedTab).toHaveCSS(
    "background-color",
    questionBackground,
  );
});

test("history slides question and answer together, reverses and handles rapid tab navigation without moving the transcript", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-eventcards--long-question-history&viewMode=story",
  );
  const card = page.locator(history);
  const slider = card.getByRole("tabpanel");
  const first = card.getByRole("tab", { name: "Q1", exact: true });
  const second = card.getByRole("tab", { name: "Q2", exact: true });
  const last = card.getByRole("tab", { name: "Q3", exact: true });
  await first.hover();
  await page.evaluate(() => document.fonts.ready);
  await slider.evaluate((node) =>
    node.style.setProperty("--ui-sliding-pages-duration", "1000ms"),
  );
  const frame = () =>
    card.evaluate((node) => {
      const box = node.getBoundingClientRect();
      const viewport = node
        .querySelector('[role="tabpanel"]')!
        .getBoundingClientRect();
      const after = document
        .querySelector('.transcript article.response[data-seq="6"]')!
        .getBoundingClientRect();
      return [
        box.width,
        box.height,
        viewport.x - box.x,
        viewport.width,
        viewport.height,
        after.y - box.y,
      ];
    });
  const before = await frame();
  const firstPage = slider.locator('[data-sliding-page="layout"]');
  const secondPage = slider.locator('[data-sliding-page="notes"]');
  await second.click();
  await expect(second).toBeFocused();
  await expect(secondPage).toHaveAttribute("data-active", "true");
  await expect.poll(() => slideX(firstPage)).toBeLessThan(-1);
  const incoming = await slideX(secondPage);
  expect(incoming).toBeGreaterThan(0);
  expect(incoming).toBeLessThan((await slider.boundingBox())!.width);
  await expect(firstPage).toHaveAttribute("inert", "");
  await expect(firstPage).toHaveAttribute("aria-hidden", "true");
  await expect(content(card, "Question")).toHaveCount(1);
  const samples = await card.evaluate(async (node) => {
    const result: number[][] = [];
    for (let index = 0; index < 5; index++) {
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => resolve()),
      );
      const box = node.getBoundingClientRect();
      const viewport = node
        .querySelector('[role="tabpanel"]')!
        .getBoundingClientRect();
      const after = document
        .querySelector('.transcript article.response[data-seq="6"]')!
        .getBoundingClientRect();
      const sections = [
        ...node.querySelectorAll("[data-sliding-page][data-active] section"),
      ].map((section) => section.getBoundingClientRect());
      result.push([
        box.width,
        box.height,
        viewport.x - box.x,
        viewport.width,
        viewport.height,
        after.y - box.y,
        sections[1].x - sections[0].x,
      ]);
    }
    return result;
  });
  for (const sample of samples) expect(sample).toEqual([...before, 0]);
  await first.click();
  await expect(firstPage).toHaveAttribute("data-active", "true");
  await expect.poll(() => slideX(firstPage)).toBeLessThan(-1);
  await expect(secondPage).toHaveAttribute("inert", "");
  await first.press("End");
  await last.press("Home");
  await first.press("ArrowRight");
  await second.press("End");
  await expect(last).toBeFocused();
  const active = slider.locator("[data-sliding-page][data-active]");
  await expect(active).toHaveAttribute("data-sliding-page", "timing");
  await expect.poll(async () => Math.abs(await slideX(active))).toBeLessThan(1);
  await expect(content(card, "Answer")).toContainText("Now");
  expect(await frame()).toEqual(before);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(errors).toEqual([]);
});

test("history switches immediately with reduced motion and retains tab focus", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(
    "/iframe.html?id=conversation-eventcards--answered-question&viewMode=story",
  );
  const card = page.locator(history);
  const second = card.getByRole("tab", { name: "Q2", exact: true });
  await expect(card).toBeVisible();
  const before = (await card.boundingBox())!.height;
  await second.click();
  const active = card.locator("[data-sliding-page][data-active]");
  await expect(active).toHaveAttribute("data-sliding-page", "notes");
  await expect(active).toHaveCSS("transition-duration", "0s");
  await expect
    .poll(async () => Math.abs(await slideX(active)))
    .toBeLessThan(0.5);
  expect(await active.evaluate((node) => node.getAnimations().length)).toBe(0);
  await expect(second).toBeFocused();
  await expect(content(card, "Answer").locator("pre code")).toHaveText(
    "const compact = true;\n",
  );
  await second.press("End");
  await expect(
    card.getByRole("tab", { name: "Q3", exact: true }),
  ).toBeFocused();
  await expect(active).toHaveAttribute("data-sliding-page", "timing");
  await expect(card.locator('[data-sliding-page="notes"]')).toHaveAttribute(
    "inert",
    "",
  );
  expect((await card.boundingBox())!.height).toBe(before);
});

for (const width of [1280, 390]) {
  test(`recorded questions stay between conversation messages at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/iframe.html?id=conversation-eventcards--question-history-in-conversation&viewMode=story",
    );
    const card = page.locator(history);
    await expect(card).toHaveCount(1);
    await expect(card).toHaveAttribute("data-state", "completed");
    await expect(
      page
        .locator(".transcript")
        .getByText("Update the composer while keeping the existing toolbar.", {
          exact: true,
        }),
    ).toBeAttached();
    await expect(
      page
        .locator(".transcript")
        .getByText(
          "The conversation retains the question and its recorded answers above. Select a question tab to review its recorded answer.",
          { exact: true },
        ),
    ).toBeAttached();
    await card.getByRole("tab", { name: "Q2", exact: true }).click();
    await expect(content(card, "Answer").locator("pre code")).toHaveText(
      "const compact = true;\n",
    );
    await expect(
      page.getByRole("button", { name: "Submit", exact: true }),
    ).toHaveCount(0);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  });

  test(`question tabs keep card, content areas and the following response fixed at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/iframe.html?id=conversation-eventcards--long-question-history&viewMode=story",
    );
    const card = page.locator(history);
    await expect(card).toBeVisible();
    await expect(card.locator("section")).toHaveCount(2);
    await page.evaluate(() => document.fonts.ready);
    const geometry = () =>
      card.evaluate((node) => {
        const origin = node.getBoundingClientRect();
        const panel = node.querySelector("[data-sliding-page][data-active]")!;
        const regions = [...panel.querySelectorAll("section")];
        const next = document.querySelector(
          '.transcript article.response[data-seq="6"]',
        )!;
        return [node, ...regions, next].map((el) => {
          const box = el.getBoundingClientRect();
          return {
            x: box.x - origin.x,
            y: box.y - origin.y,
            width: box.width,
            height: box.height,
          };
        });
      });
    const initial = await geometry();
    const textStarts = () =>
      card.evaluate((node) => {
        const next = document.querySelector(
          '.transcript article.response[data-seq="6"] > .markdown > p',
        )!;
        const expected = next.getBoundingClientRect().x;
        return [
          ...node.querySelectorAll(
            "[data-sliding-page][data-active] section .markdown > :first-child",
          ),
        ].map((el) => el.getBoundingClientRect().x - expected);
      });
    await expect.poll(textStarts).toEqual([0, 0]);
    await card.getByRole("tab", { name: "Q2", exact: true }).click();
    await expect(content(card, "Question")).toContainText("Requirement 18");
    await expect(content(card, "Answer")).toContainText("Change 12");
    await expect.poll(geometry).toEqual(initial);
    await expect.poll(textStarts).toEqual([0, 0]);
    const positions: number[] = [];
    for (const name of ["Question", "Answer"] as const) {
      const viewport = content(card, name).locator(
        '[data-scroll-area="viewport"]',
      );
      expect(
        await viewport.evaluate((el) => el.scrollHeight > el.clientHeight),
      ).toBe(true);
      await viewport.hover();
      await page.mouse.wheel(0, 160);
      await expect
        .poll(() => viewport.evaluate((el) => el.scrollTop))
        .toBeGreaterThan(0);
      await expect.poll(geometry).toEqual(initial);
      positions.push(await viewport.evaluate((el) => el.scrollTop));
    }
    await card.getByRole("tab", { name: "Q3", exact: true }).click();
    await expect(content(card, "Answer")).toContainText("Now");
    await expect.poll(geometry).toEqual(initial);
    await card.getByRole("tab", { name: "Q2", exact: true }).click();
    await expect.poll(geometry).toEqual(initial);
    for (const [index, name] of (["Question", "Answer"] as const).entries()) {
      const viewport = content(card, name).locator(
        '[data-scroll-area="viewport"]',
      );
      await expect
        .poll(() => viewport.evaluate((el) => el.scrollTop))
        .toBe(positions[index]);
    }
    await card.getByRole("tab", { name: "Q1", exact: true }).click();
    await expect(content(card, "Answer")).toContainText("Compact");
    await expect.poll(geometry).toEqual(initial);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  });
}
