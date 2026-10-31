import { expect, test } from "@playwright/test";

test("answered questions show selections and full free-text answers in their task details", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-eventcards--answered-question&viewMode=story",
  );
  const card = page.locator("button.question-history");
  await expect(card).toContainText("Choose the layout. → Compact");
  await expect(card).toContainText("Keep the existing toolbar.");
  await expect(card).toHaveAttribute("data-state", "completed");
  await card.dblclick();
  const details = page.getByRole("dialog", { name: "Question", exact: true });
  await expect(details).toContainText("Describe any additional changes.");
  await expect(details.locator("pre code")).toHaveText(
    "const compact = true;\n",
  );
  await expect(details).toContainText("Compact");
  await expect(details).toContainText("Now");
});

test("canceled questions stay in the transcript with their question text", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-eventcards--canceled-question&viewMode=story",
  );
  const card = page.locator("button.question-history");
  await expect(card).toContainText("Canceled");
  await expect(card).toHaveAttribute("data-state", "failed");
  await card.dblclick();
  await expect(
    page.getByRole("dialog", { name: "Question", exact: true }),
  ).toContainText("Choose the layout.");
});

for (const width of [1280, 390]) {
  test(`question history stays between messages and opens recorded answers at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/iframe.html?id=conversation-eventcards--question-history-in-conversation&viewMode=story",
    );
    const card = page.locator("button.question-history");
    await expect(card).toHaveCount(1);
    await expect(card).toHaveAttribute("data-state", "completed");
    await expect(card).toContainText("Choose the layout. → Compact");
    await expect(card).toContainText("When should we ship? → Now");
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
          "The conversation retains the question and its recorded answers above. Double-click its card to review every step and the complete free-text answer.",
          { exact: true },
        ),
    ).toBeAttached();
    await card.dblclick();
    const details = page.getByRole("dialog", { name: "Question", exact: true });
    await expect(details).toContainText("Describe any additional changes.");
    await expect(details.locator("pre code")).toHaveText(
      "const compact = true;\n",
    );
    await expect(details).toContainText("Compact");
    await expect(details).toContainText("Now");
    await expect(
      page.getByRole("button", { name: "Submit", exact: true }),
    ).toHaveCount(0);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  });
}
