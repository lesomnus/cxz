import { expect, test, type Locator } from "@playwright/test";
async function animations(root: Locator) {
  return root.evaluate((node) => {
    const motions = node
      .getAnimations({ subtree: true })
      .filter((animation) => animation instanceof CSSAnimation);
    motions.forEach((animation) => {
      animation.pause();
      animation.currentTime =
        Number(animation.effect!.getTiming().duration) / 2;
    });
    return motions.map(
      (animation) => (animation as CSSAnimation).animationName,
    );
  });
}
for (const reduced of [false, true]) {
  test(`Profile menus ${reduced ? "respect reduced motion" : "reveal by default"}`, async ({
    page,
  }) => {
    if (reduced) await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(
      "/iframe.html?id=workspace-errors--recovery&viewMode=story",
    );
    await page.addStyleTag({
      content: ":root { --ui-menu-reveal-duration: 2s; }",
    });
    const profile = page.getByRole("button", { name: "Profile", exact: true });
    await profile.click();
    const menu = page.getByRole("menu", { name: "Profile", exact: true });
    await expect(menu).toHaveAttribute("data-enter-from", "from-left");
    expect(
      (await animations(menu)).some((name) => name.includes("reveal")),
    ).toBe(!reduced);
    await page.keyboard.press("Escape");
    await expect(profile).toBeFocused();
  });
}

test("conversation search fades in while retaining stable geometry and focus", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-sessionsearch--default&viewMode=story",
  );
  await page.addStyleTag({
    content: ":root { --ui-dialog-enter-duration: 2s; }",
  });
  await page
    .getByRole("button", { name: "Search conversation", exact: true })
    .click();
  const search = page.getByRole("search", {
    name: "Search current conversation",
  });
  const before = await search.boundingBox();
  expect(
    (await animations(search)).some((name) => name.includes("backdrop-enter")),
  ).toBe(true);
  expect(await search.boundingBox()).toEqual(before);
  await expect(search.getByRole("searchbox")).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(search).toHaveCount(0);
});

for (const reduced of [false, true]) {
  test(`question entrance keeps its bounds and ${reduced ? "respects reduced motion" : "fades in"}`, async ({
    page,
  }) => {
    if (reduced) await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(
      "/iframe.html?id=conversation-questioncard--steps&viewMode=story",
    );
    const question = page.getByRole("region", {
      name: "Question",
      exact: true,
    });
    const before = await question.boundingBox();
    expect(
      (await animations(question)).some((name) =>
        name.includes("backdrop-enter"),
      ),
    ).toBe(!reduced);
    expect(await question.boundingBox()).toEqual(before);
  });
}
