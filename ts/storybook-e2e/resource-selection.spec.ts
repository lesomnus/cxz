import { expect, test } from "@playwright/test";

test("project and session selections survive Tasks and Memory navigation, with explicit links overriding scope", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=patterns-resource-selection--shared-scope&viewMode=story",
  );
  const project = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  const session = page.getByRole("combobox", {
    name: "Select session",
    exact: true,
  });
  const nav = page.getByRole("navigation", {
    name: "Shared scope navigation",
    exact: true,
  });
  const chooseSession = async (name: string) => {
    await session.click();
    await page.getByRole("option", { name, exact: true }).click();
  };
  const rows = page
    .getByRole("table", { name: "Tasks", exact: true })
    .locator("tbody tr[data-task-alias]");
  await expect(rows).toHaveCount(9);
  await chooseSession("UI refinement");
  await expect(rows).toHaveCount(1);
  await nav.getByRole("link", { name: "Memory", exact: true }).click();
  await expect(project).toHaveText("Workspace");
  await expect(session).toHaveText("UI refinement");
  await expect(
    page
      .getByRole("navigation", { name: "Memories", exact: true })
      .getByRole("link"),
  ).toHaveCount(4);
  await chooseSession("Task API");
  await nav.getByRole("link", { name: "Tasks", exact: true }).click();
  await expect(session).toHaveText("Task API");
  await expect(rows).toHaveCount(5);
  await chooseSession("All sessions");
  await nav.getByRole("link", { name: "Memory", exact: true }).click();
  await expect(session).toHaveText("All sessions");
  await expect(
    page
      .getByRole("navigation", { name: "Memories", exact: true })
      .getByRole("link"),
  ).toHaveCount(8);
  await chooseSession("Task API");
  await nav
    .getByRole("link", { name: "Open Documentation tasks", exact: true })
    .click();
  await expect(project).toHaveText("Documentation");
  await expect(session).toHaveText("All sessions");
  await nav.getByRole("link", { name: "Memory", exact: true }).click();
  await expect(project).toHaveText("Documentation");
  await expect(session).toHaveText("All sessions");
  await project.click();
  await page.getByRole("option", { name: "Workspace", exact: true }).click();
  await chooseSession("UI refinement");
  await nav.getByRole("link", { name: "Tasks", exact: true }).click();
  await expect(session).toHaveText("UI refinement");
  await expect(rows).toHaveCount(1);
  await nav
    .getByRole("link", { name: "Open Task API tasks", exact: true })
    .click();
  await expect(session).toHaveText("Task API");
  await expect(rows).toHaveCount(5);
  await nav.getByRole("link", { name: "Memory", exact: true }).click();
  await expect(session).toHaveText("Task API");
});
