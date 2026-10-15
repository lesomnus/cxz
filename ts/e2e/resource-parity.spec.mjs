import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
test.use({
  viewport: { width: 1280, height: 900 },
  isMobile: false,
  hasTouch: false,
});
function stream(rows) {
  const frames = rows.map((row) => {
    const body = Buffer.from(JSON.stringify(row));
    const header = Buffer.alloc(5);
    header.writeUInt32BE(body.length, 1);
    return Buffer.concat([header, body]);
  });
  return Buffer.concat([...frames, Buffer.from([2, 0, 0, 0, 2, 123, 125])]);
}
async function fixture(page, helpers = false, extraProject = false) {
  const id = (value) => Buffer.from(value.padEnd(16, "0")).toString("base64");
  const project = {
    id: id("project"),
    runtimeId: "project",
    alias: "project",
    name: "Demo project",
    workspace: "/workspace",
    listed: true,
    status: { state: "running" },
  };
  const projects = extraProject
    ? [
        project,
        {
          ...project,
          id: id("second"),
          runtimeId: "second",
          alias: "docs-playground",
          name: "Docs playground",
        },
      ]
    : [project];
  const account = {
    id: id("account"),
    alias: "work",
    name: "Work account",
    agent: "codex",
    authBackend: "brokered-access-token",
  };
  let queued = "Queued review request";
  let listed = true;
  const session = () => ({
    id: id("fixture"),
    runtimeId: "fixture",
    alias: "oak",
    name: "Research session",
    clientId: "creation",
    agent: "codex",
    listed,
    project,
    account,
    status: { state: "idle", runId: "run", lastSeq: "4", pending: [], queued },
  });
  const memory = {
    id: "own",
    session: "fixture",
    name: "Research notes",
    agent: "codex",
    size: "32",
  };
  let content = "# Shared notes\nOriginal content";
  let revision = "r1";
  const events = [
    {
      kind: "input",
      seq: "1",
      text: "Earlier question",
      runId: "run",
      timeMs: "1710000000000",
    },
    {
      kind: "assistant",
      seq: "2",
      text: "Earlier response",
      runId: "run",
      timeMs: "1710000001000",
    },
    {
      kind: "input",
      seq: "3",
      text: "Current question",
      runId: "run",
      timeMs: "1710000010000",
    },
    {
      kind: "assistant",
      seq: "4",
      text: "Current response",
      runId: "run",
      timeMs: "1710000011000",
    },
  ];
  const requests = [];
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (
      route.request().method() !== "GET" ||
      (url.pathname !== "/" && !url.pathname.startsWith("/assets/"))
    )
      return route.continue();
    const file = resolve(
      "../internal/webui/assets",
      url.pathname === "/" ? "index.html" : url.pathname.slice(1),
    );
    const contentType = file.endsWith(".js")
      ? "application/javascript"
      : file.endsWith(".css")
        ? "text/css"
        : file.endsWith(".html")
          ? "text/html"
          : "application/octet-stream";
    await route.fulfill({ body: readFileSync(file), contentType });
  });
  await page.route("**/cxz.*/*", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const raw = route.request().postDataBuffer();
    const q = raw?.length
      ? JSON.parse((raw[0] === 0 ? raw.subarray(5) : raw).toString())
      : {};
    requests.push({ path, q });
    const send = (json) => route.fulfill({ json });
    const sendStream = (rows) =>
      route.fulfill({
        contentType: "application/connect+json",
        body: stream(rows),
      });
    const error = (code, message) =>
      route.fulfill({ status: 409, json: { code, message } });
    const aux = {
      preferences: [{ kind: "AUX_KIND_SUMMARY", enabled: true }],
      recent: [],
      summaries: [],
    };
    if (path === "/cxz.ProjectService/List") return send({ items: projects });
    if (path === "/cxz.ProjectService/Watch")
      return sendStream([
        { items: projects.map((value) => ({ id: value.id, value })) },
      ]);
    if (path === "/cxz.ProjectService/Get") return send(project);
    if (path === "/cxz.AccountService/List") return send({ items: [account] });
    if (path === "/cxz.AccountService/Add")
      return send({ ...account, alias: q.alias, name: q.name, agent: q.agent });
    if (path === "/cxz.AuthBindingService/Add")
      return send({
        id: id("binding"),
        bindingId: "binding",
        account,
        project,
      });
    if (path === "/cxz.SessionService/List")
      return send({
        items:
          q.filters?.[0]?.listed === false
            ? !listed
              ? [session()]
              : []
            : listed
              ? [session()]
              : [],
      });
    if (path === "/cxz.SessionService/Watch")
      return sendStream([
        {
          items: [
            { id: session().id, value: session() },
            ...(helpers
              ? [
                  {
                    id: id("helper"),
                    value: {
                      ...session(),
                      id: id("helper"),
                      runtimeId: "helper",
                      alias: "elm",
                      name: "Helper session",
                    },
                  },
                ]
              : []),
          ],
        },
      ]);
    if (path === "/cxz.SessionService/Get") return send(session());
    if (path === "/cxz.SessionService/Add") return send(session());
    if (path === "/cxz.SessionService/Erase") {
      listed = false;
      return send({ erased: true });
    }
    if (path === "/cxz.SessionService/Restore") {
      listed = true;
      return send(session());
    }
    if (path === "/cxz.SessionService/Transcript")
      return send({
        events: events.filter(
          (e) => !q.afterSeq || Number(e.seq) > Number(q.afterSeq),
        ),
        snapshotSeq: "4",
      });
    if (path === "/cxz.SessionService/History")
      return send({
        events: events.filter((e) => Number(e.seq) > Number(q.afterSeq ?? 0)),
      });
    if (path === "/cxz.SessionService/Events") return sendStream([]);
    if (path === "/cxz.SessionService/AuxEvents") return sendStream([aux]);
    if (path === "/cxz.SessionService/AuxStatus") return send(aux);
    if (path === "/cxz.SessionService/AuxRun")
      return send({
        ...aux,
        current: {
          id: "aux1",
          state: "running",
          kinds: q.kinds,
          results: [],
          usage: [],
          turn: "4",
          responseSeq: "4",
        },
      });
    if (path === "/cxz.SessionService/AuxPrefer")
      return send({ ...aux, preferences: q.preferences });
    if (path === "/cxz.SessionService/AuxCancel")
      return send({
        ...aux,
        current: { id: "aux1", state: "canceled", results: [], usage: [] },
      });
    if (path === "/cxz.ProjectService/AuxConfig")
      return send({
        profiles: [
          {
            kind: "AUX_KIND_SUMMARY",
            enabled: true,
            account: "work",
            model: "test-model",
          },
        ],
      });
    if (path === "/cxz.ProjectService/AuxModels")
      return send({
        models: [
          { id: "test-model", name: "Test model", efforts: ["low", "high"] },
        ],
      });
    if (path === "/cxz.ProjectService/AuxSetConfig")
      return send({ profiles: q.profiles });
    if (path === "/cxz.SessionService/Models")
      return send({
        data: Buffer.from(JSON.stringify({ models: [] })).toString("base64"),
      });
    if (path === "/cxz.SessionService/Send") {
      if (q.cancel) queued = "";
      return send({ status: "accepted" });
    }
    if (path === "/cxz.SessionService/Search")
      return sendStream([
        {
          visit: {
            sessionId: "fixture",
            alias: "oak",
            title: "Research session",
            projectName: "Demo project",
            hits: [
              {
                seq: "2",
                kind: "assistant",
                timeMs: "1710000001000",
                snippet: "Earlier response",
              },
            ],
          },
        },
        { summary: { hits: 1, sessions: 1, projects: 1 } },
      ]);
    if (path === "/cxz.SessionService/LibraryMemories")
      return send({
        memories: q.scope === "LIBRARY_SCOPE_TRASH" ? [] : [memory],
      });
    if (path === "/cxz.SessionService/LibraryMemory")
      return send({
        memory,
        documents: [{ name: "notes.md", size: "32", revision }],
      });
    if (path === "/cxz.SessionService/LibraryDocument")
      return send({ memory, content, revision });
    if (path === "/cxz.SessionService/WriteLibraryDocument") {
      if (q.content.includes("conflict")) {
        revision = "r2";
        content = "# Shared notes\nExternal version";
        return error("aborted", "Document changed. Read the current revision.");
      }
      content = q.content;
      revision = "r2";
      return send({ memory, content, revision });
    }
    if (path === "/cxz.SessionService/SnapshotLibraryMemory")
      return send({ memory: { ...memory, snapshot: true } });
    if (path === "/cxz.SessionService/ConversationStats")
      return send({
        complete: true,
        total: {
          finishedTurns: "2",
          inputTokens: "100",
          outputTokens: "50",
          tokenReportedTurns: "2",
        },
      });
    if (path === "/cxz.SessionService/Background")
      return send({
        data: Buffer.from(
          JSON.stringify({
            run: {
              Tasks: {
                one: {
                  task_id: "one",
                  description: "Build assets",
                  status: "working",
                  active: true,
                },
              },
            },
          }),
        ).toString("base64"),
      });
    if (path === "/cxz.ProjectService/PutSecretFile")
      return send({ path: "/run/cxz-secrets/test" });
    if (path === "/cxz.ProjectService/DeleteSecretFile")
      return send({ path: q.path });
    return send({});
  });
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.goto("/");
  return requests;
}
test("every resource tab has its own body and navigation", async ({ page }) => {
  await fixture(page);
  for (const [name, heading] of [
    ["Projects view", "Projects"],
    ["Accounts", "Accounts"],
    ["Search", "Search"],
    ["Memory", "Shared memory"],
    ["Activity", "Activity & usage"],
    ["Auxiliary AI", "Auxiliary AI"],
  ]) {
    await page.getByRole("link", { name, exact: true }).click();
    await expect(
      name === "Projects view"
        ? page.locator("main.resource-view header strong")
        : name === "Accounts"
          ? page.getByLabel("Account list").locator("header strong")
          : page.getByRole("heading", { name: heading, exact: true }),
    ).toBeVisible();
    if (name !== "Projects view")
      await expect(page.locator(".resource-panel .project-row")).toHaveCount(0);
  }
});
test("resource bodies fit a mobile viewport", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await fixture(page);
  for (const name of ["Accounts", "Memory", "Activity"]) {
    await page.getByRole("link", { name, exact: true }).click();
    await expect(page.locator("main")).toBeVisible();
    const bounds = await page.locator("main").boundingBox();
    expect(bounds.y + bounds.height).toBeLessThanOrEqual(845);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(391);
  }
});
test("inline session mentions and secret chips preserve the prompt and storage boundary", async ({
  page,
}) => {
  const requests = await fixture(page, true);
  await page
    .getByRole("link", { name: /Research session oak/ })
    .first()
    .click();
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.pressSequentially("ask @el");
  await expect(
    page.getByRole("grid", { name: "Session mentions" }),
  ).toBeVisible();
  await input.press("Tab");
  await expect(input).toHaveValue("ask @elm ");
  await input.fill("");
  await input.pressSequentially("pass /@red");
  await input.press("Tab");
  await page
    .getByRole("dialog", { name: "Secret", exact: true })
    .locator("input[type=password]")
    .fill("synthetic-private-value");
  await page.getByRole("button", { name: "Insert secret chip" }).click();
  await expect(input).toHaveValue(/pass \[Secret [a-f0-9]{8}\]/);
  expect(
    await page.evaluate(() =>
      JSON.stringify({ ...sessionStorage, ...localStorage }),
    ),
  ).not.toContain("synthetic-private-value");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect
    .poll(() =>
      requests.some(
        (r) =>
          r.path.endsWith("SessionService/Send") &&
          r.q.text?.includes("secret placed at /run/cxz-secrets/test"),
      ),
    )
    .toBe(true);
  expect(
    requests.filter((r) => r.path.endsWith("SessionService/Send")).at(-1).q
      .text,
  ).not.toContain("synthetic-private-value");
});
test("account registration, session creation and archive restore use resource APIs", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.getByRole("link", { name: "Accounts", exact: true }).click();
  await page.getByRole("button", { name: "Add account", exact: true }).click();
  const add = page.getByRole("dialog", { name: "Add account", exact: true });
  await add
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("personal");
  await add.getByRole("button", { name: "Add", exact: true }).click();
  await expect
    .poll(() =>
      requests.some(
        (r) =>
          r.path.endsWith("AccountService/Add") && r.q.alias === "personal",
      ),
    )
    .toBe(true);
  await page.getByRole("link", { name: "Sessions view" }).click();
  const panel = page.getByRole("complementary", {
    name: "Session list",
    exact: true,
  });
  await expect(panel).not.toContainText("127.0.0.1");
  const body = panel.locator(".panel-scroll-region");
  const appearance = await panel.evaluate((el) => ({
    background: getComputedStyle(el).backgroundColor,
    body: getComputedStyle(el.querySelector(".panel-scroll-region"))
      .backgroundColor,
  }));
  expect(appearance.body).not.toBe(appearance.background);
  expect(
    await body.evaluate((el) => parseFloat(getComputedStyle(el).borderRadius)),
  ).toBeGreaterThan(0);
  await panel
    .getByRole("button", { name: "Session panel options", exact: true })
    .click();
  await page
    .getByRole("menu", { name: "Session panel options", exact: true })
    .evaluate(async (el) => {
      const shell = getComputedStyle(el);
      if (
        shell.filter !== "none" ||
        shell.maskImage !== "none" ||
        shell.transform !== "none"
      )
        throw new Error("Menu border must remain outside the reveal effects");
      const content = el.querySelector('[data-enter-effect="reveal"]');
      const animations = el.getAnimations({ subtree: true });
      if (!animations.length)
        throw new Error("Menu entrance animation missing");
      const entrance = content.getAnimations()[0];
      entrance.pause();
      entrance.currentTime = 0;
      const start = new DOMMatrix(getComputedStyle(content).transform);
      if (start.m41 >= 0 || start.m42 >= 0)
        throw new Error("Menu must reveal from the top left");
      if (getComputedStyle(content).filter !== "blur(4px)")
        throw new Error("Reveal must start blurred");
      if (getComputedStyle(content).maskImage === "none")
        throw new Error("Reveal must use a directional mask");
      entrance.currentTime = Number(entrance.effect.getTiming().duration) * 0.7;
      const settled = getComputedStyle(content);
      if (settled.filter !== "blur(0px)" || settled.maskPosition !== "0% 0%")
        throw new Error("Reveal edges must be clear before the animation ends");
      entrance.play();
      await Promise.all(animations.map((animation) => animation.finished));
    });
  const menuBox = await page
    .getByRole("menu", { name: "Session panel options", exact: true })
    .boundingBox();
  const triggerBox = await panel
    .getByRole("button", { name: "Session panel options", exact: true })
    .boundingBox();
  const panelBox = await panel.boundingBox();
  const edge = panelBox.x + panelBox.width;
  const border = await panel
    .locator("header")
    .evaluate((el) => parseFloat(getComputedStyle(el).borderRightWidth));
  expect(menuBox.y).toBeCloseTo(triggerBox.y, 0);
  expect(menuBox.x - edge).toBeCloseTo(
    edge - border - triggerBox.x - triggerBox.width,
    0,
  );
  await page
    .getByRole("menuitem", { name: "Add session", exact: true })
    .click();
  const create = page.getByRole("dialog", { name: "Add session", exact: true });
  await expect(create).toBeVisible();
  const box = await create.boundingBox();
  const viewport = page.viewportSize();
  expect(box.x + box.width / 2).toBeCloseTo(viewport.width / 2, 0);
  expect(box.y + box.height / 2).toBeCloseTo(viewport.height / 2, 0);
  await expect
    .poll(() => create.evaluate((el) => el.contains(document.activeElement)))
    .toBe(true);
  await page.keyboard.press("Escape");
  await expect(create).toHaveCount(0);
  await expect(
    panel.getByRole("button", { name: "Session panel options", exact: true }),
  ).toBeFocused();
  await page.emulateMedia({ reducedMotion: "reduce" });
  await panel
    .getByRole("button", { name: "Session panel options", exact: true })
    .click();
  const reducedMenu = page.getByRole("menu", {
    name: "Session panel options",
    exact: true,
  });
  await expect(reducedMenu).toHaveCSS("animation-name", "none");
  await expect(reducedMenu).toHaveCSS("mask-image", "none");
  await expect(reducedMenu).toHaveCSS("filter", "none");
  await expect(reducedMenu.locator('[data-enter-effect="reveal"]')).toHaveCSS(
    "animation-name",
    "none",
  );

  await page
    .getByRole("menuitem", { name: "Add session", exact: true })
    .click();
  await page.getByRole("combobox", { name: "Account", exact: true }).click();
  await page.getByRole("option", { name: "Work account · codex" }).click();
  await page
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("New title");
  await page.getByRole("button", { name: "Create session" }).click();
  await expect
    .poll(() =>
      requests.some(
        (r) =>
          r.path.endsWith("SessionService/Add") &&
          r.q.name === "New title" &&
          r.q.authBinding.bindingId === "binding",
      ),
    )
    .toBe(true);
  await expect(create).toHaveCount(0);
  await page.goto("/sessions");
  await page
    .locator("main")
    .getByRole("button", { name: "Archive", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Archive", exact: true })
    .click();
  await expect
    .poll(() => requests.some((r) => r.path.endsWith("SessionService/Erase")))
    .toBe(true);
  await page.getByRole("button", { name: "Archived", exact: true }).click();
  await page.getByRole("button", { name: "Restore", exact: true }).click();
  await expect
    .poll(() => requests.some((r) => r.path.endsWith("SessionService/Restore")))
    .toBe(true);
});
test("search opens the matched response and queue recall preserves its text", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.getByRole("link", { name: "Search", exact: true }).click();
  await page
    .getByRole("textbox", { name: "Search conversations" })
    .fill("Earlier");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await page.getByRole("link", { name: /Earlier response/ }).click();
  await expect(page).toHaveURL(/seq=%222%22/);
  await expect(
    page.getByRole("button", { name: "Recall", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Recall", exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toHaveValue("Queued review request");
  expect(
    requests.some(
      (r) => r.path.endsWith("SessionService/Send") && r.q.cancel === true,
    ),
  ).toBe(true);
});
test("conversation search uses loaded messages immediately without a Search request", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.locator('.tree-session[href="/sessions/fixture"]').click();
  await expect(page.locator(".transcript")).toContainText("Current response");
  await page.getByRole("textbox", { name: "Message", exact: true }).focus();
  await page.keyboard.press("Control+Shift+F");
  const search = page.getByRole("search", {
      name: "Search current conversation",
    }),
    input = search.getByRole("searchbox");
  await input.fill("Earlier");
  await expect(search.getByRole("status")).toHaveText("2 matches");
  await search.getByRole("button", { name: /Earlier response/ }).click();
  await expect(input).toBeFocused();
  await page.getByRole("textbox", { name: "Message", exact: true }).focus();
  await expect(search).toBeVisible();
  await input.fill("query not in loaded messages");
  await expect(search.getByRole("status")).toHaveText("No matches");
  await input.fill("Current");
  await input.fill("Earlier");
  await expect(search.getByRole("status")).toHaveText("2 matches");
  await expect(search).not.toContainText("Current response");
  expect(
    requests.filter((r) => r.path.endsWith("SessionService/Search")),
  ).toHaveLength(0);
  await input.press("Escape");
  await expect(search).toHaveCount(0);
});

test("memory edits carry the revision and keep drafts on a conflict", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.getByRole("link", { name: "Memory", exact: true }).click();
  await page.getByRole("button", { name: "notes.md", exact: true }).click();
  const editor = page.locator(".monaco-editor textarea");
  await expect(editor).toBeVisible();
  await editor.focus();
  await editor.press("Control+a");
  await editor.fill("conflict");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.locator('main p[role="alert"]')).toContainText(
    "Document changed",
  );
  expect(
    requests.find((r) => r.path.endsWith("WriteLibraryDocument"))?.q.revision,
  ).toBe("r1");
  await expect(
    page.getByText("notes.md · Unsaved", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Reload document", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Discard unsaved changes" }),
  ).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Save", exact: true }),
  ).toBeDisabled();
  await editor.focus();
  await editor.press("Control+a");
  await editor.fill("Merged notes");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect
    .poll(
      () =>
        requests.filter((r) => r.path.endsWith("WriteLibraryDocument")).at(-1)
          ?.q.revision,
    )
    .toBe("r2");
});
test("usage, background tasks and auxiliary controls are connected", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.getByRole("link", { name: "Activity", exact: true }).click();
  await expect(
    page.getByText("Complete retained coverage", { exact: true }),
  ).toBeVisible();
  await page.getByRole("combobox", { name: "Scope", exact: true }).click();
  await page
    .getByRole("option", { name: "Research session", exact: true })
    .click();
  await expect(page.getByText("Build assets", { exact: true })).toBeVisible();
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await page.getByRole("link", { name: "oak", exact: false }).first().click();
  await page.getByRole("button", { name: "Session menu" }).click();
  await page
    .getByRole("menuitem", { name: "Auxiliary AI", exact: true })
    .click();
  await page.getByRole("button", { name: "Run now" }).first().click();
  await expect
    .poll(() => requests.some((r) => r.path.endsWith("AuxRun")))
    .toBe(true);
  await expect(page.getByText("running", { exact: true })).toBeVisible();
});

test("local session search Enter visits loaded responses before wrapping without search or target history requests", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.locator('.tree-session[href="/sessions/fixture"]').click();
  await expect(page.locator(".transcript")).toContainText("Current response");
  await page.getByRole("textbox", { name: "Message", exact: true }).focus();
  await page.keyboard.press("Control+Shift+F");
  const search = page.getByRole("search", {
      name: "Search current conversation",
    }),
    input = search.getByRole("searchbox");
  await input.fill("response");
  await expect(search.getByRole("status")).toHaveText("2 matches");
  const historyCount = () =>
    requests.filter((r) => /SessionService\/(History|Transcript)$/.test(r.path))
      .length;
  const before = historyCount();
  const active = () =>
    page.evaluate(() => {
      const range = [
        ...(CSS.highlights.get("conversation-search-current") ?? []),
      ][0];
      return range?.startContainer.parentElement?.closest("[data-row]")?.dataset
        .row;
    });
  await input.press("Enter");
  await expect.poll(active).toBe("4");
  await expect(input).toBeFocused();
  await input.press("Enter");
  await expect.poll(active).toBe("2");
  await input.press("Enter");
  await expect.poll(active).toBe("4");
  expect(
    requests.filter((r) => r.path.endsWith("SessionService/Search")),
  ).toHaveLength(0);
  expect(historyCount()).toBe(before);
  await expect(input).toBeFocused();
});

test("project picker supports fuzzy search, keyboard selection and dark form backgrounds", async ({
  page,
}) => {
  await fixture(page, false, true);
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await page.getByRole("button", { name: "New session", exact: true }).click();
  const create = page
    .getByRole("dialog", { name: "Add session", exact: true })
    .or(page.getByRole("region", { name: "New session", exact: true }));
  await expect(create).toBeVisible();
  const projectPicker = create.getByRole("combobox", {
    name: "Project",
    exact: true,
  });
  const titleInput = create.getByRole("textbox", {
    name: "Title",
    exact: true,
  });
  expect(
    await projectPicker.evaluate((el) => getComputedStyle(el).backgroundColor),
  ).toBe(
    await titleInput.evaluate((el) => getComputedStyle(el).backgroundColor),
  );
  // The anchored creation dialog moves during entrance. Measure the value
  // overlay against its settled trigger, rather than an intermediate frame.
  await create.evaluate((node) =>
    Promise.all(
      node
        .getAnimations({ subtree: true })
        .filter(
          (animation) =>
            animation.effect?.getComputedTiming().endTime !== Infinity,
        )
        .map((animation) => animation.finished.catch(() => {})),
    ),
  );
  const triggerBounds = await projectPicker.boundingBox();
  await projectPicker.click();
  const search = page.getByRole("combobox", {
    name: "Search Project",
    exact: true,
  });
  await expect(search).toBeFocused();
  await expect(search).toHaveValue("Demo project");
  await search.press("Enter");
  await expect(search).toHaveCount(0);
  await expect(projectPicker).toHaveText("Demo project");
  await projectPicker.click();
  await expect(search).toHaveValue("Demo project");
  const searchBounds = await search.boundingBox();
  expect(searchBounds.x).toBeCloseTo(triggerBounds.x, 0);
  expect(searchBounds.y).toBeCloseTo(triggerBounds.y, 0);
  await expect(search).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(
    page.getByRole("option", { name: "Demo project", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("option", { name: "Docs playground", exact: true }),
  ).toBeVisible();
  await search.fill("dcsplg");
  await expect(search).toBeFocused();
  await expect(
    page.getByRole("option", { name: "Docs playground", exact: true }),
  ).toBeVisible();
  await search.fill("not-a-project");
  await expect(page.getByRole("status")).toHaveText("No matches");
  await search.press("Enter");
  await expect(search).toBeVisible();
  await search.fill("DOCS");
  await search.press("ArrowDown");
  await expect(
    page.getByRole("option", { name: "Docs playground", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(search).toHaveCount(0);
  await expect(create).toBeVisible();
  await expect(projectPicker).toBeFocused();
  await projectPicker.click();
  await expect(search).toHaveValue("Docs playground");
  await search.fill("");
  await expect(
    page.getByRole("option", { name: "Docs playground", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("option", { name: "Demo project", exact: true }),
  ).toBeVisible();
  await search.press("Escape");
  await expect(create).toBeVisible();
});
