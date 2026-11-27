import { Buffer } from "node:buffer";
import { devices, expect, test } from "@playwright/test";

test.use({ ...devices["Desktop Chrome"] });

test("project creation uses the chosen installation and a stable modal", async ({
  page,
}) => {
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.route("**/cxz.ConnectionService/List", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            name: "local",
            target: "https://local.example",
            transport: "gateway",
            isDefault: true,
          },
          {
            name: "cluster",
            target: "https://cluster.example",
            transport: "gateway",
          },
        ],
      },
    }),
  );
  await page.route("**/cxz.InstallationService/Version", (route) =>
    route.fulfill({
      json: {
        projectBackend: new URL(route.request().url()).pathname.startsWith(
          "/c/cluster/",
        )
          ? "kubernetes"
          : "docker",
      },
    }),
  );
  await page.route("**/c/cluster/auth/status", (route) =>
    route.fulfill({ status: 204 }),
  );
  const project = {
    id: Buffer.alloc(16, 7).toString("base64"),
    alias: "cluster-app",
    name: "Cluster app",
    workspace: "/workspaces/cluster-app",
    status: { containerState: "CONTAINER_STATE_ABSENT" },
  };
  await page.route("**/c/cluster/cxz.ProjectService/List", (route) =>
    route.fulfill({ json: { items: [project] } }),
  );
  await page.route("**/c/cluster/cxz.ProjectService/Get", (route) =>
    route.fulfill({ json: project }),
  );
  await page.route("**/c/cluster/cxz.SessionService/List", (route) =>
    route.fulfill({ json: { items: [] } }),
  );
  const addRequests: string[] = [];
  await page.route("**/cxz.ProjectService/Add", (route) => {
    addRequests.push(new URL(route.request().url()).pathname);
    expect(route.request().postDataJSON()).toEqual({
      workspace: "cluster-app",
      name: "Cluster app",
      config: "node:24",
    });
    return route.fulfill(
      addRequests.length === 1
        ? {
            status: 400,
            json: {
              code: "invalid_argument",
              message: "Test registration failure",
            },
          }
        : { json: project },
    );
  });

  await page.goto("/c/_/project");
  await page.getByRole("button", { name: "Add project", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add project", exact: true });
  await expect(dialog.getByLabel("Workspace path")).toBeVisible();
  await expect(dialog.getByLabel("Devcontainer configuration")).toBeVisible();
  const height = () =>
    dialog.evaluate((node) => node.getBoundingClientRect().height);
  const initial = await height();

  await dialog
    .getByRole("combobox", { name: "Connection", exact: true })
    .click();
  await page.getByRole("option", { name: "cluster", exact: true }).click();
  const projectName = dialog.getByLabel("Project name", { exact: true });
  await expect(projectName).toBeVisible();
  await expect(dialog.getByLabel("Workspace path")).toHaveCount(0);
  expect(await height()).toBe(initial);
  await projectName.fill("cluster-app");
  await dialog.getByLabel("Display name").fill("Cluster app");
  await dialog
    .getByLabel("Image or Kubernetes settings (JSON)")
    .fill("node:24");
  const confirm = dialog.getByRole("button", { name: "Confirm", exact: true });
  await confirm.click();
  await expect(
    dialog.getByText("Test registration failure", { exact: false }),
  ).toBeVisible();
  expect(await height()).toBe(initial);
  await confirm.click();
  await expect(page).toHaveURL(/\/c\/cluster\/project\/cluster-app$/);
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Cluster app");
  expect(addRequests).toEqual([
    "/c/cluster/cxz.ProjectService/Add",
    "/c/cluster/cxz.ProjectService/Add",
  ]);
  await page.getByRole("link", { name: "Projects view", exact: true }).click();
  await expect(page).toHaveURL(/\/c\/cluster\/project$/);
});
