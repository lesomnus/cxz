import { test, expect, devices, type Page, type Route } from "@playwright/test";
import { chooseSetting, expectInherited } from "./settings-controls";

test.use({
  ...devices["Desktop Chrome"],
  permissions: ["clipboard-read", "clipboard-write"],
});
// An existing dev-dependency font avoids Google/network availability in CI.
const fontPath = new URL(
  "../node_modules/storybook/assets/browser/nunito-sans-regular.woff2",
  import.meta.url,
).pathname;
const remoteFont = { provider: "google", family: "Roboto Mono" };
const cssURL = "https://fonts.googleapis.com/**";
async function respondCSS(route: Route) {
  const family = new URL(route.request().url()).searchParams
    .get("family")!
    .split(":")[0];
  await route.fulfill({
    contentType: "text/css",
    headers: { "Access-Control-Allow-Origin": "*" },
    body: `@font-face { font-family: '${family}'; font-weight: 400 700; src: url('https://fonts.gstatic.com/test-font.woff2') format('woff2'); }`,
  });
}
async function ready(page: Page) {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
}
async function settings(page: Page) {
  await page.getByRole("link", { name: "Settings view", exact: true }).click();
  await page
    .getByRole("navigation", { name: "Settings topics" })
    .getByRole("link", { name: "Editor", exact: true })
    .click();
}
function control(page: Page, scope = "Global") {
  return page.locator(".font-family-control").filter({
    has: page.getByLabel(`${scope} editor Font family`, { exact: true }),
  });
}
async function stored(page: Page) {
  return page.evaluate(() =>
    JSON.parse(localStorage.getItem("settings") ?? "{}"),
  );
}

test("downloads only on Apply, waits for font bytes, inherits and restores after reload without losing drafts", async ({
  page,
}) => {
  await page.setViewportSize({ width: 2104, height: 1000 });
  let requests = 0;
  await page.route(cssURL, async (route) => {
    requests++;
    await respondCSS(route);
  });
  let release!: () => void;
  const gate = new Promise<void>((done) => {
    release = done;
  });
  let started = false;
  await page.route("https://fonts.gstatic.com/**", async (route) => {
    started = true;
    await gate;
    await route.fulfill({
      contentType: "font/woff2",
      path: fontPath,
      headers: { "Access-Control-Allow-Origin": "*" },
    });
  });
  await ready(page);
  const composer = page.getByRole("textbox", { name: "Message", exact: true });
  await composer.fill("keep 한글 `draft`");
  await settings(page);
  expect(requests).toBe(0);
  await chooseSetting(page, "Global editor Font family", "google");
  expect(requests).toBe(0);
  await control(page)
    .getByRole("button", { name: "Apply font", exact: true })
    .click();
  await expect.poll(() => started).toBe(true);
  expect(await stored(page)).toEqual({});
  await expect(page.locator(".settings-preview").first()).not.toHaveCSS(
    "font-family",
    /Roboto Mono/,
  );
  release();
  await expect
    .poll(() => stored(page))
    .toEqual({ "editor.fontFamily": remoteFont });
  await expect(page.locator(".settings-preview")).toHaveCount(2);
  await expect(page.locator(".settings-preview").first()).toHaveCSS(
    "font-family",
    /Roboto Mono/,
  );
  await expect(page.locator(".settings-file .view-lines")).toHaveCSS(
    "font-family",
    /^"?Roboto Mono/,
  );
  await expectInherited(page, "Session editor Font family");
  expect(requests).toBe(1);
  await chooseSetting(page, "Session editor Font family", "monospace");
  await expect(page.locator(".settings-preview").last()).toHaveCSS(
    "font-family",
    "monospace",
  );
  await chooseSetting(page, "Session editor Font family", "");
  await expect(page.locator(".settings-preview").last()).toHaveCSS(
    "font-family",
    /Roboto Mono/,
  );
  await page.screenshot({ path: "test-results/google-font-settings.png" });
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(composer).toHaveValue("keep 한글 `draft`");
  await expect(composer).toHaveCSS("font-family", /Roboto Mono/);
  const actual = await composer.evaluate(
    (el) => getComputedStyle(el).fontFamily,
  );
  await expect(page.locator(".editor-mirror")).toHaveCSS("font-family", actual);
  await expect(page.locator(".editor-gutter")).toHaveCSS("font-family", actual);
  await page.reload();
  await expect(composer).toHaveCSS("font-family", /Roboto Mono/, {
    timeout: 30000,
  });
  expect(await stored(page)).toEqual({ "editor.fontFamily": remoteFont });
  expect(requests).toBe(2);
});

test("failed Apply keeps local settings and can retry; an abandoned download never overwrites a newer choice", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1300, height: 1000 });
  let fail = true;
  let release!: () => void;
  const gate = new Promise<void>((done) => {
    release = done;
  });
  let waiting = false;
  await page.route(cssURL, async (route) => {
    if (fail) {
      fail = false;
      await route.abort();
      return;
    }
    if (route.request().url().includes("JetBrains")) {
      waiting = true;
      await gate;
    }
    await respondCSS(route);
  });
  await page.route("https://fonts.gstatic.com/**", (route) =>
    route.fulfill({
      contentType: "font/woff2",
      path: fontPath,
      headers: { "Access-Control-Allow-Origin": "*" },
    }),
  );
  await ready(page);
  await settings(page);
  await chooseSetting(page, "Global editor Font family", "monospace");
  await chooseSetting(page, "Global editor Font family", "google");
  await control(page)
    .getByRole("button", { name: "Apply font", exact: true })
    .click();
  await expect(control(page).getByRole("alert")).toContainText(
    "Could not load",
  );
  expect(await stored(page)).toEqual({ "editor.fontFamily": "monospace" });
  await control(page)
    .getByRole("button", { name: "Apply font", exact: true })
    .click();
  await expect
    .poll(() => stored(page))
    .toEqual({ "editor.fontFamily": remoteFont });
  await chooseSetting(page, "Session editor Font family", "google");
  await control(page, "Session").getByRole("textbox").fill("JetBrains Mono");
  await control(page, "Session")
    .getByRole("button", { name: "Apply font", exact: true })
    .click();
  await expect.poll(() => waiting).toBe(true);
  await chooseSetting(page, "Session editor Font family", "monospace");
  release();
  await expect
    .poll(() =>
      page.evaluate(() =>
        [...document.fonts].some(
          (face) =>
            face.family.includes("JetBrains Mono") && face.status === "loaded",
        ),
      ),
    )
    .toBe(true);
  expect(await stored(page)).toEqual({
    "editor.fontFamily": remoteFont,
    "session.editor.fontFamily": "monospace",
  });
  await expect(page.locator(".settings-preview").last()).toHaveCSS(
    "font-family",
    "monospace",
  );
});

test("saved remote fonts preserve fallback after failure and Retry updates every mounted editor", async ({
  page,
}) => {
  await page.setViewportSize({ width: 2104, height: 1000 });
  await page.addInitScript((font) => {
    if (!localStorage.getItem("settings"))
      localStorage.setItem(
        "settings",
        JSON.stringify({ "editor.fontFamily": font }),
      );
  }, remoteFont);
  let fail = true;
  await page.route(cssURL, async (route) => {
    if (fail) await route.abort();
    else await respondCSS(route);
  });
  await page.route("https://fonts.gstatic.com/**", (route) =>
    route.fulfill({
      contentType: "font/woff2",
      path: fontPath,
      headers: { "Access-Control-Allow-Origin": "*" },
    }),
  );
  await ready(page);
  await settings(page);
  await expect(control(page).getByRole("alert")).toContainText(
    "Could not load",
  );
  await expect(page.locator(".settings-file .view-lines")).not.toHaveCSS(
    "font-family",
    /Roboto Mono/,
  );
  fail = false;
  await control(page)
    .getByRole("button", { name: "Retry", exact: true })
    .click();
  await expect(page.locator(".settings-file .view-lines")).toHaveCSS(
    "font-family",
    /^"?Roboto Mono/,
  );
  await expect(page.locator(".settings-preview").last()).toHaveCSS(
    "font-family",
    /Roboto Mono/,
  );
  await expect(control(page).getByRole("alert")).toHaveCount(0);
  expect(await stored(page)).toEqual({ "editor.fontFamily": remoteFont });
});
