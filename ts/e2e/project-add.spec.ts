import { Buffer } from "node:buffer";
import { devices, expect, test } from "@playwright/test";

test.use({ ...devices["Desktop Chrome"] });

const defaultKubeConfig = {
  image: "ghcr.io/lesomnus/dev-golang:1.27",
  user: "hypnos",
};

test("project creation uses the chosen installation and a stable modal", async ({
  page,
}) => {
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  const clusterId = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
  const clusterRPC = `/c/${clusterId}`;
  await page.route("**/cxz.ConnectionService/List", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            name: "local",
            target: new URL(route.request().url()).origin,
            transport: "origin",
            isDefault: true,
          },
          {
            id: Buffer.from(clusterId.replaceAll("-", ""), "hex").toString(
              "base64",
            ),
            name: "cluster",
            target: "https://cluster.example",
            transport: "gateway",
          },
          {
            name: "legacy",
            target: "https://legacy.example",
            transport: "gateway",
          },
        ],
      },
    }),
  );
  await page.route("**/cxz.InstallationService/Version", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.startsWith("/c/@legacy/")) return route.fulfill({ json: {} });
    return route.fulfill({
      json: {
        projectBackend: path.startsWith(`${clusterRPC}/`)
          ? "kubernetes"
          : "docker",
      },
    });
  });
  await page.route(`**/c/{${clusterId},@legacy}/auth/status`, (route) =>
    route.fulfill({ status: 204 }),
  );
  const project = {
    id: Buffer.alloc(16, 7).toString("base64"),
    alias: "cluster-app",
    name: "Cluster app",
    workspace: "/workspaces/cluster-app",
    status: { containerState: "CONTAINER_STATE_ABSENT" },
  };
  await page.route(`**${clusterRPC}/cxz.ProjectService/List`, (route) =>
    route.fulfill({ json: { items: [project] } }),
  );
  await page.route(`**${clusterRPC}/cxz.ProjectService/Get`, (route) =>
    route.fulfill({ json: project }),
  );
  await page.route(`**${clusterRPC}/cxz.SessionService/List`, (route) =>
    route.fulfill({ json: { items: [] } }),
  );
  const addRequests: string[] = [];
  await page.route("**/cxz.ProjectService/Add", (route) => {
    addRequests.push(new URL(route.request().url()).pathname);
    const submitted = route.request().postDataJSON();
    expect(submitted).toMatchObject({
      workspace: "cluster-app",
      name: "Cluster app",
    });
    if (addRequests.length === 1)
      expect(JSON.parse(submitted.config)).toEqual(defaultKubeConfig);
    else expect(submitted.config).toBe("node:24");
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

  await page.goto("/c/@local/project");
  await page.getByRole("button", { name: "Add project", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add project", exact: true });
  const workspace = dialog.getByLabel("Workspace path");
  await expect(workspace).toBeVisible();
  await expect(dialog.getByLabel("Devcontainer configuration")).toHaveValue("");
  await workspace.focus();
  const ringClearance = await workspace.evaluate((input) => {
    let viewport = input.parentElement;
    while (viewport && getComputedStyle(viewport).overflowY !== "auto")
      viewport = viewport.parentElement;
    if (!viewport) throw new Error("Missing project form scroll viewport");
    const { outlineWidth, outlineOffset } = getComputedStyle(input);
    const ring = parseFloat(outlineWidth) + parseFloat(outlineOffset);
    const field = input.getBoundingClientRect();
    const scroll = viewport.getBoundingClientRect();
    return (
      Math.min(field.left - scroll.left, scroll.right - field.right) - ring
    );
  });
  expect(ringClearance).toBeGreaterThan(0);
  const height = () =>
    dialog.evaluate((node) => node.getBoundingClientRect().height);
  const initial = await height();
  expect(initial).toBeGreaterThan(500);

  const connection = dialog.getByRole("combobox", {
    name: "Connection",
    exact: true,
  });
  const confirm = dialog.getByRole("button", { name: "Confirm", exact: true });
  await connection.click();
  await page.getByRole("option", { name: "legacy", exact: true }).click();
  await expect(dialog.getByRole("alert")).toHaveText(
    "Update the connected manager to report its project runtime.",
  );
  await expect(dialog.getByRole("combobox")).toHaveCount(1);
  await expect(confirm).toBeDisabled();
  expect(await height()).toBe(initial);

  await connection.click();
  await page.getByRole("option", { name: "cluster", exact: true }).click();
  const projectName = dialog.getByLabel("Project name", { exact: true });
  await expect(projectName).toBeVisible();
  await expect(dialog.getByLabel("Workspace path")).toHaveCount(0);
  const config = dialog.getByLabel("Image or Kubernetes settings (JSON)");
  await expect(config).toHaveValue(JSON.stringify(defaultKubeConfig, null, 2));
  expect(await height()).toBe(initial);
  await projectName.fill("cluster-app");
  await dialog.getByLabel("Display name").fill("Cluster app");
  await confirm.click();
  await expect(
    dialog.getByText("Test registration failure", { exact: false }),
  ).toBeVisible();
  expect(await height()).toBe(initial);
  await config.fill("node:24");
  await confirm.click();
  await expect(page).toHaveURL(/\/c\/@cluster\/project\/cluster-app$/);
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Cluster app");
  expect(addRequests).toEqual([
    `${clusterRPC}/cxz.ProjectService/Add`,
    `${clusterRPC}/cxz.ProjectService/Add`,
  ]);
  await page.goto(`${clusterRPC}/project/cluster-app`);
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Cluster app");
  await page.getByRole("link", { name: "Projects view", exact: true }).click();
  await expect(page).toHaveURL(/\/c\/@cluster\/project$/);
});
