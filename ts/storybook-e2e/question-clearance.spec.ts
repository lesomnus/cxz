import { expect, test } from "@playwright/test";

test("a question reserves its tucked height and releases it after answering", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--long-questions&viewMode=story",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  await expect(question).toBeVisible();
  const pane = page.locator(".transcript");
  const area = page.locator(".transcript-area");
  const layer = page.locator(".question-cards");
  const clearance = () =>
    area.evaluate((el) => parseFloat(getComputedStyle(el).marginBottom));
  await expect.poll(clearance).toBeGreaterThan(50);
  const reserved = await clearance();
  const height = await pane.evaluate((el) => el.clientHeight);
  await expect
    .poll(() =>
      pane.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop),
    )
    .toBeLessThan(2);
  const bounds = (await area.boundingBox())!;
  await page.mouse.move(bounds.x + 10, bounds.y + 40);
  await pane.evaluate((el) =>
    el
      .closest(".conversation")!
      .dispatchEvent(new Event("conversation-reading-move")),
  );
  await expect(layer).toHaveAttribute("data-retreated", "true");
  await expect
    .poll(async () => {
      const card = (await question.boundingBox())!;
      const viewport = (await area.boundingBox())!;
      return Math.abs(viewport.y + viewport.height - card.y);
    })
    .toBeLessThan(2);
  // Revealing a card must not resize or move the transcript.
  await question.getByRole("radio", { name: "Compact", exact: true }).check();
  await expect.poll(clearance).toBeCloseTo(reserved, 0);
  const cancel = question.getByRole("button", { name: "Cancel", exact: true });
  await cancel.click();
  await question
    .getByRole("button", { name: "Confirm cancel", exact: true })
    .click();
  await expect(question).toHaveCount(0);
  await expect.poll(clearance).toBe(0);
  await expect
    .poll(async () =>
      Math.abs(
        (await pane.evaluate((el) => el.clientHeight)) - height - reserved,
      ),
    )
    .toBeLessThan(1);
  await expect
    .poll(() =>
      pane.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop),
    )
    .toBeLessThan(2);
});
