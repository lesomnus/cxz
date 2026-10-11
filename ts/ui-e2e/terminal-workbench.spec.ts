import { test, expect } from "@playwright/test";

test("portable terminal workbench preserves renderer state across docking without app providers", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-terminalworkbench--portable&viewMode=story",
  );
  const panel = page.getByRole("region", { name: "Workspace terminal" });
  await expect(panel).toContainText("Renderer preview");
  const input1 = page.getByRole("textbox", { name: "Input terminal-1" });
  await input1.fill("keep-first-draft");
  await panel.getByRole("button", { name: "New terminal" }).click();
  const input2 = page.getByRole("textbox", { name: "Input terminal-2" });
  await input2.fill("keep-second-history");
  await input2.press("Enter");
  await input2.fill("keep-second-draft");
  const slot = panel.getByRole("tabpanel");
  const bounds = (await slot.boundingBox())!;
  await panel.locator('[data-terminal-tab="terminal-2"]').dragTo(slot, {
    targetPosition: { x: bounds.width - 8, y: bounds.height / 2 },
  });
  await expect(panel.locator("[data-terminal-group]")).toHaveCount(2);
  await expect(input1).toHaveValue("keep-first-draft");
  await expect(input2).toHaveValue("keep-second-draft");
  await expect(panel).toContainText("keep-second-history");
  await expect(
    panel.getByRole("button", { name: "Terminal layout" }),
  ).toHaveCount(0);
  await panel.getByRole("button", { name: "Hide terminal" }).click();
  await expect(panel).toBeHidden();
  await page.getByRole("button", { name: "Toggle terminal" }).click();
  await expect(input2).toHaveValue("keep-second-draft");
});
