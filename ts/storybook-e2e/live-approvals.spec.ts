import { expect, test } from "@playwright/test";

test("automatic approvals do not flash cards; unresolved approvals and questions stay actionable", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
  );
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toBeVisible();
  await expect(group).toHaveAttribute("data-pending-count", "0");
  await expect(group).toHaveAttribute("aria-busy", "false");
  await expect(
    group.getByRole("button", { name: "Deny", exact: true }),
  ).toBeDisabled();
  await expect(
    group.getByRole("button", { name: "Allow", exact: true }),
  ).toBeDisabled();
  const idleBounds = await group.boundingBox();
  await page.locator(".question-cards").waitFor({ state: "attached" });
  await page.evaluate(() => {
    const seen: string[] = [];
    (window as any).approvalFlashes = seen;
    const observer = new MutationObserver(() => {
      for (const card of document.querySelectorAll(
        ".question-cards .approval, [data-approval-stack], [role='group'][data-pending-count]:not([data-pending-count='0'])",
      ))
        seen.push(card.getAttribute("aria-label") ?? "approval");
    });
    observer.observe(document.querySelector(".conversation")!, {
      childList: true,
      subtree: true,
    });
  });
  await page
    .getByRole("button", { name: "Simulate automatic approvals" })
    .click();
  await page.waitForTimeout(1800);
  expect(await page.evaluate(() => (window as any).approvalFlashes)).toEqual(
    [],
  );
  await page
    .getByRole("button", { name: "Request approval", exact: true })
    .click();
  await expect(group).toHaveAttribute("data-pending-count", "1");
  await expect(page.locator("[data-approval-stack]")).toHaveCount(0);
  await group.hover();
  await expect(
    page.getByRole("region", {
      name: "item/commandExecution/requestApproval",
      exact: true,
    }),
  ).toBeVisible();
  expect(await group.boundingBox()).toEqual(idleBounds);
  await expect(
    page.getByRole("group", { name: "Pending approvals", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Resolve request", exact: true })
    .click();
  await expect(page.locator(".question-cards .approval")).toHaveCount(0);
  await expect(page.locator("[data-approval-stack]")).toHaveCount(0);
  await expect(
    group.getByRole("button", { name: "Deny", exact: true }),
  ).toBeDisabled();
  await expect(
    group.getByRole("button", { name: "Allow", exact: true }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "Ask question", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Question", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Resolve request", exact: true })
    .click();
  await expect(page.locator(".question-cards .approval")).toHaveCount(0);
  expect(errors).toEqual([]);
});

for (const width of [1280, 390]) {
  test(`toolbar approvals stack independent requests and resolve one at a time at ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 850 });
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(
      "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
    );
    const request = page.getByRole("button", {
      name: "Request approval",
      exact: true,
    });
    await request.click();
    await request.click();
    await request.click();
    const group = page.getByRole("group", {
      name: "Pending approvals",
      exact: true,
    });
    const stack = page.locator("[data-approval-stack]");
    const current = page.getByRole("region", {
      name: "item/commandExecution/requestApproval",
      exact: true,
    });
    await expect(group).toHaveAttribute("data-pending-count", "3");
    await group.hover();
    await expect(stack).toHaveAttribute("data-positioned", "true");
    await expect(stack.locator("[data-approval-back]")).toHaveCount(2);
    await expect(current).toContainText("git status --short");
    await expect(current).toContainText("/workspace");
    await expect(current.locator(".card-heading, .card-footer")).toHaveCount(0);
    await expect(
      group.getByRole("button", { name: "Deny", exact: true }),
    ).toBeEnabled();
    await expect(
      group.getByRole("button", { name: "Allow", exact: true }),
    ).toBeEnabled();
    const buttons = (await group.boundingBox())!;
    const menu = (await page
      .getByRole("button", { name: "Session menu", exact: true })
      .boundingBox())!;
    const cards = (await stack.boundingBox())!;
    const conversation = (await page.locator(".conversation").boundingBox())!;
    expect(buttons.x + buttons.width).toBeLessThanOrEqual(menu.x);
    expect(cards.y + cards.height).toBeLessThan(buttons.y);
    expect(cards.width).toBeLessThanOrEqual(320);
    expect(cards.x).toBeGreaterThanOrEqual(conversation.x);
    expect(cards.x + cards.width).toBeLessThanOrEqual(
      conversation.x + conversation.width,
    );
    if (width === 1280)
      expect(cards.x + cards.width / 2).toBeCloseTo(
        buttons.x + buttons.width / 2,
        0,
      );
    await current.evaluate(async (node) => {
      await Promise.all(
        node
          .getAnimations({ subtree: true })
          .filter((a) => a.effect?.getTiming().iterations !== Infinity)
          .map((a) => a.finished.catch(() => {})),
      );
    });
    await page.screenshot({
      path: testInfo.outputPath("stacked-approvals.png"),
    });
    await current.locator("pre code").evaluate((node) => {
      node.textContent =
        "printf 'A long permission request stays inside the card'\n".repeat(80);
    });
    await expect
      .poll(async () => (await current.boundingBox())!.height)
      .toBeLessThanOrEqual(360);
    await current.hover();
    await expect(
      current.getByRole("scrollbar", { name: "Request details", exact: true }),
    ).toBeVisible();
    expect((await stack.boundingBox())!.y).toBeGreaterThanOrEqual(
      conversation.y,
    );
    await group.getByRole("button", { name: "Allow", exact: true }).click();
    await expect(group).toHaveAttribute("data-pending-count", "2");
    await expect(current).toContainText("npm run build");
    await group.getByRole("button", { name: "Deny", exact: true }).click();
    await expect(group).toHaveAttribute("data-pending-count", "1");
    await expect(current).toContainText("git diff --stat");
    await expect(stack.locator("[data-approval-back]")).toHaveCount(0);
    await group.getByRole("button", { name: "Allow", exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(group).toHaveAttribute("data-pending-count", "0");
    await expect(
      group.getByRole("button", { name: "Allow", exact: true }),
    ).toBeDisabled();
    await expect(
      group.getByRole("button", { name: "Deny", exact: true }),
    ).toBeDisabled();
    await expect(stack).toHaveCount(0);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    expect(errors).toEqual([]);
  });
}

test("permission decisions preserve a separate pending Question", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
  );
  await page
    .getByRole("button", { name: "Request approval", exact: true })
    .click();
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toBeVisible();
  await page.getByRole("button", { name: "Ask question", exact: true }).click();
  const question = page.getByRole("region", { name: "Question", exact: true });
  await expect(question).toBeVisible();
  await group.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(group).toHaveAttribute("data-pending-count", "0");
  await expect(
    group.getByRole("button", { name: "Allow", exact: true }),
  ).toBeDisabled();
  await expect(question).toBeVisible();
  await question.getByRole("radio", { name: /^Conversation / }).check();
  await question.getByRole("button", { name: "Submit", exact: true }).click();
  await expect(question).toHaveCount(0);
});

test("a busy approval keeps the request visible and both decisions disabled", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story&args=busy:true",
  );
  await page
    .getByRole("button", { name: "Request approval", exact: true })
    .click();
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toHaveAttribute("aria-busy", "true");
  await group.hover();
  await expect(
    group.getByRole("button", { name: "Allow", exact: true }),
  ).toBeDisabled();
  await expect(
    group.getByRole("button", { name: "Deny", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("region", {
      name: "item/commandExecution/requestApproval",
      exact: true,
    }),
  ).toBeVisible();
});

test("approval count stays visible while request cards open on hover and keyboard focus", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
  );
  const controls = page.locator("[data-approval-controls]");
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  const request = page.getByRole("button", {
    name: "Request approval",
    exact: true,
  });
  await request.click();
  await request.click();
  await expect(group).toHaveAttribute("data-pending-count", "2");
  const count = controls.getByRole("img", {
    name: "Pending approvals: 2",
    exact: true,
  });
  await expect(count).toBeVisible();
  const countBox = (await count.boundingBox())!;
  const groupBox = (await group.boundingBox())!;
  const denyBox = (await group
    .getByRole("button", { name: "Deny", exact: true })
    .boundingBox())!;
  expect(countBox.x).toBeGreaterThan(groupBox.x);
  expect(countBox.x + countBox.width).toBeLessThan(denyBox.x);
  expect(
    await count.evaluate((node) => node.parentElement?.getAttribute("role")),
  ).toBe("group");
  const divider = (await group
    .locator(":scope > [aria-hidden='true']")
    .boundingBox())!;
  expect(divider.x).toBeGreaterThan(countBox.x + countBox.width);
  expect(divider.x + divider.width).toBeLessThan(denyBox.x);
  const alignments = await group.evaluate((node) => {
    const original = node.style.getPropertyValue("--ui-font-mono");
    try {
      return ["monospace", "serif"].map((font) => {
        node.style.setProperty("--ui-font-mono", font);
        const bounds = node.getBoundingClientRect();
        const count = node.querySelector<HTMLElement>("[data-mode='integer']")!;
        const textStyle = getComputedStyle(count);
        return {
          font,
          textHeight: count.getBoundingClientRect().height,
          lineHeight: textStyle.lineHeight,
          fontSize: textStyle.fontSize,
          childHeights: [...count.children].map(
            (child) => child.getBoundingClientRect().height,
          ),
          buttons: [...node.querySelectorAll("button")].map((button) => {
            const box = button.getBoundingClientRect();
            const icon = button.querySelector("svg")!.getBoundingClientRect();
            return {
              top: box.top - bounds.top,
              bottom: bounds.bottom - box.bottom,
              center:
                icon.top + icon.height / 2 - (bounds.top + bounds.height / 2),
            };
          }),
        };
      });
    } finally {
      if (original) node.style.setProperty("--ui-font-mono", original);
      else node.style.removeProperty("--ui-font-mono");
    }
  });
  for (const measurement of alignments) {
    expect(measurement.lineHeight, measurement.font).toBe(measurement.fontSize);
    for (const height of [measurement.textHeight, ...measurement.childHeights])
      expect(Number.isInteger(height), measurement.font).toBe(true);
    for (const button of measurement.buttons) {
      expect(button.top, measurement.font).toBeCloseTo(button.bottom, 5);
      expect(button.center, measurement.font).toBeCloseTo(0, 5);
    }
  }
  const stack = page.locator("[data-approval-stack]");
  await expect(stack).toHaveCount(0);
  await controls.hover();
  const current = page.getByRole("region", {
    name: "item/commandExecution/requestApproval",
    exact: true,
  });
  await expect(current).toBeVisible();
  await current.hover();
  await expect(current).toBeVisible();
  await page.mouse.move(10, 200);
  await expect(stack).toHaveCount(0);
  await expect(group).toHaveAttribute("data-pending-count", "2");
  await group.getByRole("button", { name: "Deny", exact: true }).focus();
  await page.keyboard.press("Tab");
  await expect(current).toBeVisible();
});

test("approval queue animates arrival, promotion and departure without keeping stale requests", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
  );
  await page.locator("[data-approval-controls]").waitFor();
  await page.evaluate(() => {
    const seen = new WeakSet<Element>();
    const motions: { kind: string; frames: Keyframe[] }[] = [];
    (window as any).approvalMotions = motions;
    new MutationObserver(() => {
      for (const node of document.querySelectorAll(
        "[data-approval-stack] [data-motion], [data-approval-exiting], [data-approval-back] > div",
      )) {
        if (seen.has(node)) continue;
        seen.add(node);
        motions.push({
          kind: node.hasAttribute("data-approval-exiting")
            ? "depart"
            : (node.getAttribute("data-motion") ?? "back"),
          frames: node
            .getAnimations()
            .flatMap((a) => (a.effect as KeyframeEffect).getKeyframes()),
        });
      }
    }).observe(document.querySelector(".conversation")!, {
      childList: true,
      subtree: true,
    });
  });
  const request = page.getByRole("button", {
    name: "Request approval",
    exact: true,
  });
  await request.click();
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toHaveAttribute("data-pending-count", "1");
  await group.hover();
  await expect(page.locator("[data-approval-stack]")).toBeVisible();
  // The demo shortcut appends requests without moving the pointer away.
  await page.keyboard.press("Control+Alt+n");
  await expect(group).toHaveAttribute("data-pending-count", "2");
  await page.keyboard.press("Control+Alt+n");
  await expect(group).toHaveAttribute("data-pending-count", "3");
  await page.keyboard.press("Control+Alt+n");
  await expect(group).toHaveAttribute("data-pending-count", "4");
  await group.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(group).toHaveAttribute("data-pending-count", "3");
  await expect(
    page.getByRole("region", {
      name: "item/commandExecution/requestApproval",
      exact: true,
    }),
  ).toContainText("npm run build");
  const motions = await page.evaluate(
    () =>
      (window as any).approvalMotions as { kind: string; frames: Keyframe[] }[],
  );
  for (const kind of ["arrive", "back", "promote", "depart"]) {
    const motion = motions.find((m) => m.kind === kind);
    expect(motion, kind).toBeTruthy();
    expect(motion!.frames.length, kind).toBeGreaterThan(1);
    expect(motion!.frames[0].transform, kind).not.toEqual(
      motion!.frames.at(-1)!.transform,
    );
  }
  await group.getByRole("button", { name: "Deny", exact: true }).click();
  await expect(group).toHaveAttribute("data-pending-count", "2");
  await group.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(group).toHaveAttribute("data-pending-count", "1");
  await group.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(group).toHaveAttribute("data-pending-count", "0");
  await expect(page.locator("[data-approval-stack]")).toHaveCount(0);
  await expect(page.locator("[data-approval-exiting]")).toHaveCount(0);
});

test("approval queue respects reduced motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
  );
  const request = page.getByRole("button", {
    name: "Request approval",
    exact: true,
  });
  await request.click();
  await request.click();
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toHaveAttribute("data-pending-count", "2");
  await group.hover();
  await expect(page.locator("[data-approval-stack]")).toBeVisible();
  expect(
    await page
      .locator("[data-motion]")
      .evaluateAll((nodes) => nodes.flatMap((n) => n.getAnimations()).length),
  ).toBe(0);
  await group.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(group).toHaveAttribute("data-pending-count", "1");
  await expect(page.locator("[data-approval-exiting]")).toHaveCount(0);
});

test("approval shortcut is documented and isolated inside the full Storybook manager", async ({
  page,
}) => {
  await page.goto("/?path=/story/conversation-questioncard--live-approvals");
  const preview = page.frameLocator("#storybook-preview-iframe");
  await expect(preview.getByText(/to add an approval request\./)).toBeVisible();
  await preview
    .getByRole("button", { name: "Request approval", exact: true })
    .click();
  const group = preview.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toHaveAttribute("data-pending-count", "1");
  await group.hover();
  await preview.locator("body").evaluate(() => {
    (window as any).approvalShortcutLeaks = 0;
    window.addEventListener("keydown", (event) => {
      if (event.code === "KeyN" && event.ctrlKey && event.altKey)
        (window as any).approvalShortcutLeaks++;
    });
  });
  await page.keyboard.press("Control+Alt+n");
  await expect(group).toHaveAttribute("data-pending-count", "2");
  expect(
    await preview
      .locator("body")
      .evaluate(() => (window as any).approvalShortcutLeaks),
  ).toBe(0);
  await expect(page).toHaveURL(
    /path=\/story\/conversation-questioncard--live-approvals/,
  );
});

test("departing approvals stay opaque behind the composer before the next card comes forward", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
  );
  await page
    .getByRole("button", { name: "Request approval", exact: true })
    .click();
  await page.keyboard.press("Control+Alt+n");
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toHaveAttribute("data-pending-count", "2");
  await group.hover();
  await page
    .getByRole("region", {
      name: "item/commandExecution/requestApproval",
      exact: true,
    })
    .evaluate(async (node) => {
      await Promise.all(
        node.getAnimations().map((a) => a.finished.catch(() => {})),
      );
    });
  await page.evaluate(() => {
    const seen = new WeakSet<Element>();
    const observer = new MutationObserver(() => {
      for (const node of document.querySelectorAll(
        "[data-approval-exiting], [data-motion='promote']",
      )) {
        if (seen.has(node)) continue;
        seen.add(node);
        for (const animation of node.getAnimations()) {
          animation.pause();
          animation.currentTime = 0;
        }
      }
    });
    observer.observe(document.querySelector(".conversation")!, {
      childList: true,
      subtree: true,
    });
    (window as any).approvalPauseObserver = observer;
  });
  await group.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(group).toHaveAttribute("data-pending-count", "1");
  const sample = await page.evaluate(() => {
    const outgoing = document.querySelector<HTMLElement>(
      "[data-approval-exiting]",
    )!;
    const incoming = document.querySelector<HTMLElement>(
      "[data-motion='promote']",
    )!;
    const stack = document.querySelector<HTMLElement>("[data-approval-stack]")!;
    const composer = document.querySelector<HTMLElement>(".composer")!;
    const input = composer.querySelector<HTMLElement>(".composer-input")!;
    const departure = outgoing.getAnimations()[0];
    const promotion = incoming.getAnimations()[0];
    const delay = Number(promotion.effect!.getTiming().delay);
    departure.currentTime = delay / 2;
    promotion.currentTime = delay / 2;
    const before = {
      opacity: getComputedStyle(outgoing).opacity,
      promotionProgress: promotion.effect!.getComputedTiming().progress,
      outgoingLayer: Number(getComputedStyle(outgoing.parentElement!).zIndex),
      incomingLayer: Number(getComputedStyle(incoming).zIndex),
      stackLayer: Number(getComputedStyle(stack).zIndex),
      composerLayer: Number(getComputedStyle(composer).zIndex),
    };
    departure.currentTime = delay;
    promotion.currentTime = delay;
    const hiddenBeforePromotion =
      outgoing.getBoundingClientRect().bottom >
      input.getBoundingClientRect().top;
    departure.currentTime =
      Number(departure.effect!.getTiming().duration) - 0.01;
    const end = {
      opacity: getComputedStyle(outgoing).opacity,
      top: outgoing.getBoundingClientRect().top,
      inputTop: input.getBoundingClientRect().top,
      clipBottom: outgoing.parentElement!.getBoundingClientRect().bottom,
      inputBottom: input.getBoundingClientRect().bottom,
    };
    (window as any).approvalPauseObserver.disconnect();
    departure.finish();
    promotion.finish();
    return {
      delay,
      before,
      hiddenBeforePromotion,
      end,
      departureDuration: Number(departure.effect!.getTiming().duration),
      promotionDuration: Number(promotion.effect!.getTiming().duration),
    };
  });
  expect(sample.delay).toBeGreaterThan(0);
  expect(sample.promotionDuration).toBeGreaterThan(sample.departureDuration);
  expect(sample.before.opacity).toBe("1");
  expect(sample.before.promotionProgress).toBe(0);
  expect(sample.before.outgoingLayer).toBeGreaterThan(
    sample.before.incomingLayer,
  );
  expect(sample.before.stackLayer).toBeGreaterThan(sample.before.composerLayer);
  expect(sample.hiddenBeforePromotion).toBe(true);
  expect(sample.end.opacity).toBe("1");
  expect(sample.end.top).toBeGreaterThanOrEqual(sample.end.inputTop);
  expect(sample.end.clipBottom).toBeLessThanOrEqual(sample.end.inputBottom + 1);
  await expect(page.locator("[data-approval-exiting]")).toHaveCount(0);
  await expect(
    page.getByRole("region", {
      name: "item/commandExecution/requestApproval",
      exact: true,
    }),
  ).toContainText("npm run build");
});

test("hover enters from the front and dismisses from the back with the front last", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
  );
  await page
    .getByRole("button", { name: "Request approval", exact: true })
    .click();
  await page.keyboard.press("Control+Alt+n");
  await page.keyboard.press("Control+Alt+n");
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toHaveAttribute("data-pending-count", "3");
  await page.evaluate(() => {
    const seen = new WeakSet<Animation>();
    const observer = new MutationObserver(() => {
      for (const node of document.querySelectorAll(
        "[data-motion], [data-approval-exiting], [data-approval-back] > div",
      )) {
        for (const animation of node.getAnimations()) {
          if (seen.has(animation)) continue;
          seen.add(animation);
          animation.pause();
          animation.currentTime = 0;
        }
      }
    });
    observer.observe(document.querySelector(".conversation")!, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ["data-closing"],
    });
    (window as any).approvalCascadeObserver = observer;
  });
  await group.hover();
  const stack = page.locator("[data-approval-stack]");
  await expect(stack).toHaveAttribute("data-closing", "false");
  const opening = await stack.evaluate((node) => {
    const cards = [
      node.querySelector("[data-motion]")!,
      ...node.querySelectorAll("[data-approval-back] > div"),
    ];
    const timing = cards.map((card) =>
      card.getAnimations()[0].effect!.getTiming(),
    );
    for (const card of cards)
      for (const animation of card.getAnimations()) animation.finish();
    return timing.map((t) => Number(t.delay));
  });
  expect(opening[1]).toBeGreaterThan(opening[0]);
  expect(opening[2]).toBeGreaterThan(opening[1]);
  await page.mouse.move(10, 200);
  await expect(stack).toHaveAttribute("data-closing", "true");
  const closing = await stack.evaluate((node) => {
    const cards = [
      node.querySelector("[data-approval-exiting]")!,
      ...node.querySelectorAll("[data-approval-back] > div"),
    ];
    return cards.map((card) => {
      const t = card.getAnimations()[0].effect!.getTiming();
      return {
        delay: Number(t.delay),
        end: Number(t.delay) + Number(t.duration),
      };
    });
  });
  expect(closing[2].delay).toBe(0);
  expect(closing[1].delay).toBeGreaterThan(closing[2].delay);
  expect(closing[0].delay).toBeGreaterThan(closing[1].delay);
  expect(closing[1].end).toBeLessThan(closing[0].end);
  expect(closing[2].end).toBeLessThan(closing[1].end);
  await stack
    .locator("[data-approval-back='2'] > div")
    .evaluate((node) => node.getAnimations().forEach((a) => a.finish()));
  await expect(stack).toBeVisible();
  await stack
    .locator("[data-approval-back='1'] > div")
    .evaluate((node) => node.getAnimations().forEach((a) => a.finish()));
  await expect(stack).toBeVisible();
  await stack
    .locator("[data-approval-exiting]")
    .evaluate((node) => node.getAnimations().forEach((a) => a.finish()));
  await expect(stack).toHaveCount(0);
  await expect(group).toHaveAttribute("data-pending-count", "3");
  await page.evaluate(() =>
    (window as any).approvalCascadeObserver.disconnect(),
  );
  await group.hover();
  await expect(
    page.getByRole("region", {
      name: "item/commandExecution/requestApproval",
      exact: true,
    }),
  ).toContainText("git status --short");
});

test("full queued cards remain above Question and uncover their bodies while closing", async ({
  page,
}, testInfo) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
  );
  await page.getByRole("button", { name: "Ask question", exact: true }).click();
  const question = page.getByRole("region", { name: "Question", exact: true });
  await expect(question).toBeVisible();
  await page
    .getByRole("button", { name: "Request approval", exact: true })
    .click();
  await page.keyboard.press("Control+Alt+n");
  await page.keyboard.press("Control+Alt+n");
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toHaveAttribute("data-pending-count", "3");
  await group.hover();
  const stack = page.locator("[data-approval-stack]");
  await stack.evaluate(async (node) => {
    await Promise.all(
      [...node.querySelectorAll("[data-motion], [data-approval-back] > div")]
        .flatMap((n) => n.getAnimations())
        .map((a) => a.finished.catch(() => {})),
    );
  });
  await expect(stack.locator("[data-approval-back='1']")).toContainText(
    "npm run build",
  );
  await expect(stack.locator("[data-approval-back='2']")).toContainText(
    "git diff --stat",
  );
  const before = await stack.evaluate((node) => ({
    layer: getComputedStyle(node).zIndex,
    backHeight: node
      .querySelector("[data-approval-back] > div")!
      .getBoundingClientRect().height,
    frontHeight: node.querySelector("[data-motion]")!.getBoundingClientRect()
      .height,
  }));
  expect(before.backHeight).toBeGreaterThanOrEqual(before.frontHeight - 1);
  await page.evaluate(() => {
    const seen = new WeakSet<Animation>();
    const observer = new MutationObserver(() => {
      if (!document.querySelector("[data-approval-stack][data-closing='true']"))
        return;
      for (const node of document.querySelectorAll(
        "[data-approval-exiting], [data-approval-back] > div",
      ))
        for (const animation of node.getAnimations()) {
          if (seen.has(animation)) continue;
          seen.add(animation);
          animation.pause();
          animation.currentTime = 0;
        }
    });
    observer.observe(document.querySelector(".conversation")!, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ["data-closing"],
    });
    (window as any).approvalFullBackObserver = observer;
  });
  await page.mouse.move(10, 200);
  await expect(stack).toHaveAttribute("data-closing", "true");
  const sample = await stack.evaluate(async (node) => {
    const departure = node.querySelector<HTMLElement>(
      "[data-approval-exiting]",
    )!;
    const backs = [
      ...node.querySelectorAll<HTMLElement>("[data-approval-back] > div"),
    ];
    // Hold the deeper face before retreat to inspect its full uncovered body.
    for (const [index, face] of backs.entries())
      for (const a of face.getAnimations()) {
        const timing = a.effect!.getTiming();
        a.currentTime =
          Number(timing.delay) +
          Number(timing.duration) * (index === 0 ? 0.25 : 0);
      }
    for (const a of departure.getAnimations())
      a.currentTime =
        Number(a.effect!.getTiming().delay) +
        Number(a.effect!.getTiming().duration) * 0.75;
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
    );
    const deepest = backs[1];
    const style = getComputedStyle(deepest);
    const covered = Number(style.clipPath.match(/[-\d.]+/g)?.[2] ?? 0);
    const composer = node
      .closest(".conversation")!
      .querySelector<HTMLElement>(".composer-wrapper")!;
    const box = deepest.getBoundingClientRect();
    return {
      layer: getComputedStyle(node).zIndex,
      questionLayer: getComputedStyle(
        document.querySelector(".floating-card-host")!,
      ).zIndex,
      visibleHeight: parseFloat(style.height) - covered,
      fullHeight: parseFloat(style.height),
      border: style.borderBottomWidth,
      clipBottom: deepest.parentElement!.getBoundingClientRect().bottom,
      composerTop: composer.getBoundingClientRect().top,
      overlapsQuestion:
        box.top <
        document.querySelector(".question-cards")!.getBoundingClientRect()
          .bottom,
    };
  });
  expect(sample.layer).toBe(before.layer);
  expect(Number(sample.layer)).toBeGreaterThan(Number(sample.questionLayer));
  expect(sample.visibleHeight).toBeGreaterThan(sample.fullHeight / 2);
  expect(sample.border).not.toBe("0px");
  expect(sample.clipBottom).toBeLessThanOrEqual(sample.composerTop + 1);
  expect(sample.overlapsQuestion).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("closing-full-cards-over-question.png"),
  });
  await stack.evaluate((node) => {
    (window as any).approvalFullBackObserver.disconnect();
    for (const face of node.querySelectorAll(
      "[data-approval-exiting], [data-approval-back] > div",
    ))
      for (const a of face.getAnimations()) a.finish();
  });
  await expect(stack).toHaveCount(0);
  await expect(question).toBeVisible();
  await expect(group).toHaveAttribute("data-pending-count", "3");
});
