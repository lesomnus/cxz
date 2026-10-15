import { expect, test } from "@playwright/test";

for (const story of [
  "final-response",
  "commentary",
  "claude-response",
  "long-model-name",
]) {
  test(`${story}: recorded time sits below model/effort with space between unclipped rows`, async ({
    page,
  }) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(
      `/iframe.html?id=conversation-eventcards--${story}&viewMode=story`,
    );
    const response = page.locator("article.response");
    const header = response.locator(".response-heading");
    const settings = header.locator(".response-settings");
    const time = header.locator("time");
    await expect(time).toBeVisible();
    await expect(time).toHaveAttribute(
      "title",
      "Time cxz received and recorded the response",
    );
    await expect(time).toHaveAttribute("datetime", /^\d{4}-\d{2}-\d{2}T/);
    await expect(time).not.toContainText(new Date().getFullYear().toString());
    await expect(response.locator(".response-footer time")).toHaveCount(0);
    for (const width of [1280, 390]) {
      await page.setViewportSize({ width, height: 900 });
      const headingBox = (await header.boundingBox())!;
      const copyBox = (await header.locator("button.copy").boundingBox())!;
      const modelBox = (await settings.boundingBox())!;
      const timeBox = (await header.locator(".response-time").boundingBox())!;
      const logoBox = (await header.locator(".agent-brand").boundingBox())!;
      expect(copyBox.height).toBe(24);
      expect(headingBox.height).toBeGreaterThanOrEqual(copyBox.height);
      expect(modelBox.x).toBe(timeBox.x);
      expect(timeBox.y).toBeGreaterThan(modelBox.y + modelBox.height);
      expect(modelBox.x).toBeGreaterThan(logoBox.x + logoBox.width);
      expect(timeBox.y + timeBox.height).toBeLessThanOrEqual(
        headingBox.y + headingBox.height,
      );
      await expect(settings).toHaveCSS("overflow-y", "visible");
      await expect(header.locator(".response-time")).toHaveCSS(
        "overflow-y",
        "visible",
      );
      if (story === "long-model-name") {
        await expect(settings).toHaveCSS("text-overflow", "ellipsis");
        expect(
          await settings.evaluate(
            (node) => node.scrollWidth > node.clientWidth,
          ),
        ).toBe(true);
      }
    }
    await expect(header.locator(".response-relative-time")).toHaveText(
      /^ - .*ago$/,
    );
    expect(
      await header.evaluate(
        (node) =>
          getComputedStyle(node.querySelector(".response-time")!).color !==
          getComputedStyle(node.querySelector(".response-settings")!).color,
      ),
    ).toBe(true);
    const timestamp = await time.getAttribute("datetime");
    await response.hover();
    await expect(
      header.getByRole("button", { name: "Copy", exact: true }),
    ).toBeVisible();
    await expect(time).toHaveAttribute("datetime", timestamp!);
    expect(errors).toEqual([]);
  });
}

test("assistant timestamps share input relative-age boundaries and omit expired ages", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-eventcards--recent-response&viewMode=story",
  );
  await expect(page.locator(".response-relative-time")).toHaveText(
    " - 5 minutes ago",
  );
  await page.goto(
    "/iframe.html?id=conversation-eventcards--older-response&viewMode=story",
  );
  await expect(page.locator(".response-heading time")).toBeVisible();
  await expect(page.locator(".response-relative-time")).toHaveCount(0);
  await page.goto(
    "/iframe.html?id=conversation-eventcards--user-input&viewMode=story",
  );
  await expect(page.locator(".input-relative-time")).toHaveText(/^- .+$/);
  await expect(page.locator(".input-relative-time")).not.toContainText("(");
});
