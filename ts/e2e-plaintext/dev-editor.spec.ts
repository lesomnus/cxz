import { test, expect, devices } from "@playwright/test";

// The editor panel is laid out beside the conversation, so it needs the width
// `editor.spec.ts` gives its production counterpart.
test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 2000, height: 1000 },
});

// Opening a file is the one thing in this app that loads a dependency nothing
// else needs: monaco, behind the editor's lazy `import("./code-editor")`. On a
// dev server that import used to be where vite first saw monaco, so it
// re-optimized, every pre-bundled chunk's hash changed, and the imports already
// in flight answered 504 Outdated Optimize Dep -- the file never opened and the
// page reported "Failed to fetch dynamically imported module". The production
// bundle has no such step, which is why `editor.spec.ts` is named for it and
// this had to be its own test, run against a real dev server. See cxz#232.
//
// The assertion is that nothing answered with an outdated dependency, because
// the error a person sees is the symptom and the stale chunk is the cause.
test("a file opens from the explorer on a dev server without a stale dependency", async ({
  page,
  baseURL,
}) => {
  const outdated: string[] = [];
  page.on("response", (response) => {
    if (response.status() === 504) outdated.push(response.url());
  });
  await page.request.post("/auth/login", {
    headers: { Origin: new URL(baseURL!).origin },
    data: { token: "a".repeat(32) },
  });
  await page.goto("/sessions/session");
  const editor = page.getByRole("complementary", { name: "Workspace editor" });
  await expect(editor).toBeVisible({ timeout: 30000 });
  await editor.getByRole("button", { name: "README.md", exact: true }).click();
  await expect(editor.locator(".view-lines")).toContainText(
    "Fixture workspace",
    { timeout: 60000 },
  );
  expect(outdated).toEqual([]);
});
