import { expect, test, type Locator } from "@playwright/test";

const history = "article.response.question-history";

function content(card: Locator, name: "Question" | "Answer") {
  return card.getByRole("group", { name, exact: true });
}

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

test("live and recorded Question cards share tab geometry with separate selection styles", async ({
  page,
}) => {
  const appearance = (tab: Locator) =>
    tab.evaluate((node) => {
      const style = getComputedStyle(node);
      const circle = node.querySelector("circle")!;
      const check = node.querySelector("path")!;
      const icon = node.querySelector(".tab-answered")!;
      const label = node.querySelector(".tab-label")!;
      return {
        width: style.width,
        height: style.height,
        padding: style.padding,
        font: style.font,
        columns: getComputedStyle(node.querySelector(".button-content")!)
          .gridTemplateColumns,
        iconWidth: icon.getBoundingClientRect().width,
        labelWidth: label.getBoundingClientRect().width,
        label: label.textContent,
        circleColor: getComputedStyle(circle).stroke,
        circleWidth: getComputedStyle(circle).strokeWidth,
        checkColor: getComputedStyle(check).stroke,
        checkWidth: getComputedStyle(check).strokeWidth,
      };
    });
  await page.goto(
    "/iframe.html?id=conversation-questioncard--steps&viewMode=story",
  );
  const live = page.getByRole("region", { name: "Question", exact: true });
  const liveTab = live.getByRole("tab", { name: "Q1", exact: true });
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
  await expect.poll(() => appearance(recordedTab)).toEqual(expected);
  await expect(recordedTab).toHaveCSS("border-radius", "0px");
  await expect(recordedTab).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(recordedTab).toHaveCSS("border-bottom-style", "solid");
  await expect(recordedTab).toHaveAttribute("data-effect", "none");
  await recordedTab.hover();
  await expect(recordedTab).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
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
    await page.evaluate(() => document.fonts.ready);
    const geometry = () =>
      card.evaluate((node) => {
        const origin = node.getBoundingClientRect();
        const panel = node.querySelector('[role="tabpanel"]')!;
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
          ...node.querySelectorAll("section .markdown > :first-child"),
        ].map((el) => el.getBoundingClientRect().x - expected);
      });
    await expect.poll(textStarts).toEqual([0, 0]);
    await card.getByRole("tab", { name: "Q2", exact: true }).click();
    await expect(content(card, "Question")).toContainText("Requirement 18");
    await expect(content(card, "Answer")).toContainText("Change 12");
    await expect.poll(geometry).toEqual(initial);
    await expect.poll(textStarts).toEqual([0, 0]);
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
    }
    await card.getByRole("tab", { name: "Q3", exact: true }).click();
    await expect(content(card, "Answer")).toContainText("Now");
    await expect.poll(geometry).toEqual(initial);
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
