import { Buffer } from "node:buffer";
import { devices, expect, test } from "@playwright/test";

test.use({ ...devices["Desktop Chrome"] });

const defaultKubeConfig = {
  image: "ghcr.io/lesomnus/dev-golang:1.27",
  user: "hypnos",
};

test("project creation uses the chosen installation and a stable modal", async ({
  baseURL,
  page,
}) => {
  await page.addInitScript(() => {
    if (!localStorage.getItem("settings"))
      localStorage.setItem("settings", JSON.stringify({ "ui.theme": "dark" }));
  });
  await page.request.post("/auth/login", {
    headers: { Origin: new URL(baseURL!).origin },
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
    workspace: "07070707-0707-0707-0707-070707070707",
    status: {
      containerState: "CONTAINER_STATE_ABSENT",
      remoteWorkspace: "/workspace",
    },
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
      workspace: "/workspace",
      name: "Cluster app",
      alias: "cluster-app",
    });
    expect(JSON.parse(submitted.config)).toEqual(
      addRequests.length === 1
        ? defaultKubeConfig
        : {
            image: "node:24",
            user: "hypnos",
            storage: "8Gi",
            requests: { cpu: "1" },
            pod_templates: ["gpu"],
          },
    );
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
  await expect(
    connection.locator('[data-icon="connection-docker"]'),
  ).toBeVisible();
  const confirm = dialog.getByRole("button", { name: "Confirm", exact: true });
  await connection.click();
  const legacy = page.getByRole("option", { name: "legacy", exact: true });
  await expect(
    legacy.locator('[data-icon="connection-gateway"]'),
  ).toBeVisible();
  await legacy.click();
  await expect(dialog.getByRole("alert")).toHaveText(
    "Update the connected manager to report its project runtime.",
  );
  await expect(dialog.getByRole("combobox")).toHaveCount(1);
  await expect(confirm).toBeDisabled();
  expect(await height()).toBe(initial);

  await connection.click();
  const cluster = page.getByRole("option", { name: "cluster", exact: true });
  const logo = cluster.locator('img[data-icon="connection-kubernetes"]');
  await expect(logo).toHaveAttribute("src", /kubernetes-icon-white.*\.svg$/);
  await expect(logo).not.toHaveJSProperty("naturalWidth", 0);
  await cluster.click();
  await expect(dialog.getByLabel("Project name", { exact: true })).toHaveCount(
    0,
  );
  await expect(dialog.getByLabel("Workspace path")).toHaveCount(0);
  const image = dialog.getByLabel("Container image", { exact: true });
  const user = dialog.getByLabel("Container user", { exact: true });
  const mode = dialog.getByRole("radiogroup", { name: "Kubernetes settings" });
  await expect(image).toHaveValue(defaultKubeConfig.image);
  await expect(user).toHaveValue(defaultKubeConfig.user);
  expect(await height()).toBe(initial);
  await dialog.getByLabel("Display name").fill("Cluster app");
  await dialog.getByLabel("Alias", { exact: true }).fill("cluster-app");
  await confirm.click();
  await expect(
    dialog.getByText("Test registration failure", { exact: false }),
  ).toBeVisible();
  expect(await height()).toBe(initial);
  await mode.getByRole("radio", { name: "YAML" }).check();
  const editor = dialog.getByRole("textbox", {
    name: /Kubernetes project YAML/,
  });
  await expect(editor).toBeVisible();
  await expect(dialog.locator(".view-lines")).toContainText(
    defaultKubeConfig.image,
  );
  await editor.press("Control+End");
  await page.keyboard.insertText("sto");
  await editor.press("Control+Space");
  await expect(dialog.locator(".suggest-widget.visible")).toContainText(
    "storage",
  );
  await editor.press("Escape");
  await editor.press("Control+a");
  await page.keyboard.insertText("requests: [");
  await expect(confirm).toBeDisabled();
  await expect(
    dialog.getByText("Invalid Kubernetes settings.", { exact: false }),
  ).toBeVisible();
  await mode.getByRole("radio", { name: "Form" }).check();
  await expect(confirm).toBeDisabled();
  await mode.getByRole("radio", { name: "YAML" }).check();
  await expect(dialog.locator(".view-lines")).toContainText("requests:");
  await editor.press("Control+a");
  await editor.evaluate((input, text) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", text);
    input.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  }, "# deployment settings\nimage: alpine\nuser: hypnos\nstorage: 8Gi\nrequests:\n  cpu: '1'\npod_templates: [gpu]\n");
  await expect(confirm).toBeEnabled();
  await mode.getByRole("radio", { name: "Form" }).check();
  await expect(image).toHaveValue("alpine");
  await expect(dialog.getByLabel("CPU request")).toHaveValue("1");
  await image.fill("node:24");
  await mode.getByRole("radio", { name: "YAML" }).check();
  await expect(dialog.locator(".view-lines")).toContainText("node:24");
  await expect(dialog.locator(".view-lines")).toContainText(
    "# deployment settings",
  );
  expect(await height()).toBe(initial);
  await confirm.click();
  await expect(page).toHaveURL(/\/c\/@cluster\/project\/cluster-app$/);
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Cluster app");
  const workspacePath = page.getByRole("button", {
    name: "Copy: Workspace",
    exact: true,
  });
  await expect(workspacePath).toBeVisible();
  await expect(workspacePath).toContainText("/workspace");
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

  await page.goto("/setting/connection");
  const connections = page.getByRole("table", {
    name: "Connections",
    exact: true,
  });
  await expect(
    connections
      .getByRole("rowheader", { name: /^local/ })
      .locator('[data-icon="connection-docker"]'),
  ).toBeVisible();
  const settingsLogo = connections
    .getByRole("rowheader", { name: "cluster", exact: true })
    .locator('img[data-icon="connection-kubernetes"]');
  await expect(settingsLogo).toBeVisible();
  await expect(
    connections
      .getByRole("rowheader", { name: "legacy", exact: true })
      .locator('[data-icon="connection-gateway"]'),
  ).toBeVisible();

  await page.evaluate(() =>
    localStorage.setItem("settings", JSON.stringify({ "ui.theme": "light" })),
  );
  await page.reload();
  await expect(settingsLogo).toHaveAttribute(
    "src",
    /kubernetes-icon-black.*\.svg$/,
  );
  await expect(settingsLogo).not.toHaveJSProperty("naturalWidth", 0);
});
