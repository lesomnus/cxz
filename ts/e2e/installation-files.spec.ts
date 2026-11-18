import { devices, expect, test } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1280, height: 900 },
  permissions: ["clipboard-read", "clipboard-write"],
});

test("installation files edit in Monaco and apply only to the active host", async ({
  page,
  baseURL,
}) => {
  expect((await page.request.get("/configuration/compose")).status()).toBe(401);
  const auth = await page.request.post("/auth/login", {
    headers: { Origin: baseURL! },
    data: { token: "a".repeat(32) },
  });
  expect(auth.ok()).toBe(true);
  await page.goto("/setting/connection");
  const settings = page.getByRole("region", { name: "Installation files" });
  await expect(settings).toContainText(baseURL!);
  const before = await page.evaluate(() => localStorage.getItem("settings"));
  await settings.getByRole("button", { name: "Compose override" }).click();
  const compose = page.getByRole("dialog", { name: "Compose override" });
  const editor = compose.locator(".monaco-editor");
  await expect(editor).toBeVisible();
  const existing = await (
    await page.request.get("/configuration/compose")
  ).json();
  const marker = "# From the installation editor";
  const yaml = Buffer.from(
    existing.files["docker-compose.yaml"].data,
    "base64",
  ).toString("utf8");
  await page.evaluate(
    (value) => navigator.clipboard.writeText(value),
    yaml + "\n" + marker + "\n",
  );
  const input = compose.getByRole("textbox", { name: "docker-compose.yaml" });
  await input.press("Control+a");
  await input.press("Control+v");
  await expect(editor.locator(".view-lines")).toContainText(marker);
  const draft = await (await page.request.get("/configuration/compose")).json();
  expect(draft).toEqual(existing);
  await compose.getByRole("button", { name: "Apply changes" }).click();
  await expect(compose).toHaveCount(0);
  const saved = await (await page.request.get("/configuration/compose")).json();
  expect(
    Buffer.from(saved.files["docker-compose.yaml"].data, "base64").toString(
      "utf8",
    ),
  ).toContain(marker);
  expect(await page.evaluate(() => localStorage.getItem("settings"))).toBe(
    before,
  );

  await settings
    .getByRole("button", { name: "Default devcontainer template" })
    .click();
  const template = page.getByRole("dialog", {
    name: "Default devcontainer template",
  });
  await expect(template.locator(".monaco-editor")).toBeVisible();
  await template
    .getByRole("textbox", { name: "New file path" })
    .fill("scripts/setup.sh");
  await template.getByRole("button", { name: "Add file" }).click();
  await template.getByRole("switch", { name: "Executable" }).click();
  await page.evaluate(() => navigator.clipboard.writeText("#!/bin/sh\n"));
  await template
    .getByRole("textbox", { name: "scripts/setup.sh" })
    .press("Control+v");
  await expect(template.locator(".monaco-editor .view-lines")).toContainText(
    "#!/bin/sh",
  );
  await template.getByRole("button", { name: "Apply changes" }).click();
  await expect(template).toHaveCount(0);
  const files = (
    await (await page.request.get("/configuration/template")).json()
  ).files;
  expect(files["scripts/setup.sh"]).toEqual({
    data: "IyEvYmluL3NoCg==",
    executable: true,
  });
});
