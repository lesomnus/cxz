import { test, expect } from "@playwright/test";
import { disableTerminalWebGL } from "../test-support/terminal";

// The desktop path serves http on a loopback address and no certificate. What
// that has to prove is a browser rule, not ours: Chrome treats 127.0.0.1 as a
// secure context, so the __Host- session cookie is accepted and survives a
// reload. If it ever stops being accepted, sign-in silently fails and this test
// is how we find out.
test("sign in over loopback http, stay signed in, sign out", async ({
  page,
}) => {
  await disableTerminalWebGL(page);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await expect(
    page.getByRole("button", { name: /Demo project/ }),
  ).toBeVisible();

  const named = async () =>
    (await page.context().cookies()).find((c) => c.name === "__Host-cxz");
  const cookie = await named();
  expect(cookie, "no __Host- cookie was stored").toBeDefined();
  expect(cookie?.httpOnly).toBe(true);
  expect(cookie?.sameSite).toBe("Strict");

  // A reload carries the cookie: the session is the server's, not the tab's.
  await page.reload();
  await expect(
    page.getByRole("button", { name: /Demo project/ }),
  ).toBeVisible();

  // HSTS is a promise about a host; a loopback gateway must not make one.
  const response = await page.goto("/");
  expect(response?.headers()["strict-transport-security"]).toBeUndefined();

  await page.getByRole("link", { name: /demo-chat/ }).click();
  await page.keyboard.press("Control+Backquote");
  const terminal = page.getByRole("region", { name: "Workspace terminal" });
  await expect(terminal).toContainText("fixture$");
  await expect(terminal.locator(".xterm-helper-textarea")).toBeFocused();
  await page.keyboard.type("printf 'plaintext:%s\\n' shell");
  await page.keyboard.press("Enter");
  await expect(terminal).toContainText("plaintext:shell");

  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(
    page.getByRole("button", { name: "Connect", exact: true }),
  ).toBeVisible();
  expect(await named()).toBeUndefined();
  expect(errors).toEqual([]);
});
