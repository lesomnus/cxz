import { expect, test } from "@playwright/test";

test("automatic approvals do not flash cards; unresolved approvals and questions stay actionable", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
  );
  await page.locator(".question-cards").waitFor({ state: "attached" });
  await page.evaluate(() => {
    const seen: string[] = [];
    (window as any).approvalFlashes = seen;
    const observer = new MutationObserver(() => {
      for (const card of document.querySelectorAll(".question-cards .approval"))
        seen.push(card.getAttribute("aria-label") ?? "approval");
    });
    observer.observe(document.querySelector(".question-cards")!, {
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
  await expect(
    page.getByRole("region", {
      name: "item/commandExecution/requestApproval",
      exact: true,
    }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Resolve request", exact: true })
    .click();
  await expect(page.locator(".question-cards .approval")).toHaveCount(0);
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
