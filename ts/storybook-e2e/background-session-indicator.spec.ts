import { expect, test } from "@playwright/test";

test("sessions requiring Resume are dimmed without stale activity indicators", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=sessions-panel--lifecycle-states&viewMode=story",
  );
  for (const state of ["stopped", "interrupted", "failed"]) {
    const row = page.locator(`.tree-session[data-state="${state}"]`);
    await expect(row.locator(".session-heading")).toHaveCSS("opacity", "0.25");
    await expect(row.locator(".session-activity-dot")).toHaveCount(0);
  }
  // A fatal/error notification must remain visible even when the run is gone.
  await expect(
    page
      .locator('.tree-session[data-state="interrupted"]')
      .getByTitle("Unread error"),
  ).toBeVisible();
  for (const state of ["idle", "working"]) {
    await expect(
      page.locator(`.tree-session[data-state="${state}"] .session-heading`),
    ).toHaveCSS("opacity", "1");
  }
  // Completion notifications survive for live, idle sessions.
  await expect(
    page
      .locator('.tree-session[data-state="idle"]')
      .getByTitle("Unread response"),
  ).toBeVisible();
});

test("idle background work uses dim working dots while unread completion takes priority", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=sessions-panel--background-only&viewMode=story",
  );
  const background = page.getByRole("img", {
    name: "Background tasks",
    exact: true,
  });
  await expect(background.locator("circle")).toHaveCount(8);
  await expect(background.locator("svg")).toHaveCSS("opacity", "0.4");
  await expect(
    page.getByRole("img", { name: "working", exact: true }).locator("svg"),
  ).toHaveCSS("opacity", "1");
  await expect(
    page.locator('.tree-session[data-state="stopped"] .session-indicator svg'),
  ).toHaveCount(0);
  await page.goto(
    "/iframe.html?id=sessions-panel--background-with-unread&viewMode=story",
  );
  await expect(
    page.getByRole("img", { name: "Unread response", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("img", { name: "Background tasks", exact: true }),
  ).toHaveCount(0);
});

test("working sessions retain distinct seeded phases and colors after remount and refresh", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=sessions-panel--working-phases&viewMode=story",
  );
  const rows = page.locator(".tree-session");
  await expect(rows).toHaveCount(8);
  const appearance = () =>
    rows.evaluateAll((nodes) =>
      nodes.map((row) =>
        [
          ...row.querySelectorAll<SVGCircleElement>(".session-activity-dot"),
        ].map((dot) => ({
          delay: Number(dot.getAnimations()[0]?.effect?.getTiming().delay),
          duration: Number(
            dot.getAnimations()[0]?.effect?.getComputedTiming().duration,
          ),
          color: dot.style.getPropertyValue("--ui-activity-dot-highlight"),
        })),
      ),
    );
  const initial = await appearance();
  expect(new Set(initial.map((dots) => dots[0].delay)).size).toBe(
    initial.length,
  );
  expect(
    new Set(initial.map((dots) => dots.map((dot) => dot.color).join(","))).size,
  ).toBeGreaterThan(1);
  for (const dots of initial) {
    expect(dots).toHaveLength(8);
    for (let i = 1; i < dots.length; i++)
      expect(dots[i].delay - dots[i - 1].delay).toBeCloseTo(
        -dots[i].duration / dots.length,
        2,
      );
  }
  await page.locator(".tree-project").click();
  await expect(rows).toHaveCount(0);
  await page.locator(".tree-project").click();
  await expect(rows).toHaveCount(8);
  expect(await appearance()).toEqual(initial);
  await page.reload();
  await expect(rows).toHaveCount(8);
  expect(await appearance()).toEqual(initial);
});

test("seeded dots keep the grow/shrink motion and respect reduced-motion preferences", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=sessions-panel--working-phases&viewMode=story",
  );
  const activity = page.locator(".tree-session .session-indicator svg").first();
  await expect(activity).toBeVisible();
  const samples = await activity.evaluate((svg) => {
    const dot = svg.querySelector("circle")!;
    const animation = dot.getAnimations()[0];
    animation.pause();
    const duration = Number(animation.effect!.getComputedTiming().duration);
    const delay = Number(animation.effect!.getTiming().delay);
    const start = Math.ceil(-delay / duration) * duration + delay;
    const scales = [0, 1 / 16, 1 / 8].map((phase) => {
      animation.currentTime = start + duration * phase;
      return new DOMMatrix(getComputedStyle(dot).transform).a;
    });
    animation.cancel();
    return scales;
  });
  expect(samples[0]).toBeCloseTo(0, 5);
  expect(samples[1]).toBeGreaterThan(0);
  expect(samples[1]).toBeLessThan(1);
  expect(samples[2]).toBeCloseTo(1, 5);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect
    .poll(() =>
      page
        .locator(".session-activity-dot")
        .evaluateAll((dots) =>
          dots.reduce((count, dot) => count + dot.getAnimations().length, 0),
        ),
    )
    .toBe(0);
});
