import { test, expect } from "@playwright/test";
import { BinaryWriter, WireType } from "@bufbuild/protobuf/wire";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { projectFixture } from "../test-support/projects.ts";
import { expectAPIError } from "../test-support/client-errors.ts";

test.use({
  viewport: { width: 1280, height: 900 },
  isMobile: false,
  hasTouch: false,
});
function stream(rows) {
  return Buffer.concat([
    ...rows.map((row) => {
      const body = Buffer.from(JSON.stringify(row));
      const header = Buffer.alloc(5);
      header.writeUInt32BE(body.length, 1);
      return Buffer.concat([header, body]);
    }),
    Buffer.from([2, 0, 0, 0, 2, 123, 125]),
  ]);
}
function loginDetail(sessionKey = "original-creation", account = "work1") {
  const w = new BinaryWriter()
    .tag(1, WireType.LengthDelimited)
    .string("PROJECT_LOGIN_REQUIRED")
    .tag(2, WireType.LengthDelimited)
    .string("cxz.auth");
  for (const [key, value] of [
    ["account", account],
    ["session_key", sessionKey],
  ])
    w.tag(3, WireType.LengthDelimited).bytes(
      new BinaryWriter()
        .tag(1, WireType.LengthDelimited)
        .string(key)
        .tag(2, WireType.LengthDelimited)
        .string(value)
        .finish(),
    );
  return {
    type: "google.rpc.ErrorInfo",
    value: Buffer.from(w.finish()).toString("base64"),
  };
}
async function fixture(
  page,
  { state = "idle", authFailure = true, forkError, resumeError } = {},
) {
  const id = (v) => Buffer.from(v.padEnd(16, "0")).toString("base64");
  const project = {
    id: projectFixture().id,
    alias: "project",
    name: "Demo project",
    workspace: "/workspace",
    status: { containerState: "CONTAINER_STATE_RUNNING" },
  };
  const initialAccount = {
    id: id("account"),
    alias: "work1",
    name: "Claude account",
    agent: "claude",
    authBackend: "project-local-oauth",
  };
  const nextAccount = {
    ...initialAccount,
    id: id("account2"),
    alias: "work2",
    name: "Second Claude account",
  };
  let account = initialAccount;
  let run = "original-run";
  let authenticated = false;
  let child;
  const requests = [];
  const sockets = [];
  const session = () => ({
    id: id("session"),
    runtimeId: "fixture",
    alias: "seal",
    name: "Claude session",
    clientId: "original-creation",
    listed: true,
    agent: "claude",
    project,
    account,
    status: { state, runId: run, lastSeq: "3", pending: [] },
  });
  const events = [
    {
      seq: "1",
      kind: "input",
      text: "Please continue",
      runId: run,
      timeMs: "1710000000000",
    },
    {
      seq: "2",
      kind: "assistant",
      text: "Failed to authenticate: OAuth session expired and could not be refreshed",
      runId: run,
      timeMs: "1710000001000",
    },
  ];
  const metadata = [
    {
      seq: "3",
      kind: "turn_end",
      runId: run,
      payload: Buffer.from(
        JSON.stringify({ authentication_required: authFailure }),
      ).toString("base64"),
    },
  ];
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
    await route.fulfill({
      body: readFileSync(file),
      contentType: file.endsWith(".js")
        ? "application/javascript"
        : file.endsWith(".css")
          ? "text/css"
          : file.endsWith(".html")
            ? "text/html"
            : "application/octet-stream",
    });
  });
  await page.route("**/cxz.*/*", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/cxz.ConnectionService/List") return route.continue();
    const raw = route.request().postDataBuffer();
    const q = raw?.length
      ? JSON.parse((raw[0] === 0 ? raw.subarray(5) : raw).toString())
      : {};
    requests.push({ path, q });
    const send = (json) => route.fulfill({ json });
    const frames = (rows) =>
      route.fulfill({
        contentType: "application/connect+json",
        body: stream(rows),
      });
    const needsLogin = (key) =>
      route.fulfill({
        status: 400,
        json: {
          code: "failed_precondition",
          message: `account ${account.alias} needs Claude subscription login`,
          details: [loginDetail(key, account.alias)],
        },
      });
    for (const [service, value] of [
      ["Project", project],
      ["Account", account],
      ["Session", session()],
    ]) {
      const isSession = service === "Session";
      const items =
        service === "Account"
          ? [initialAccount, nextAccount]
          : isSession && child
            ? [value, child]
            : [value];
      if (path === `/cxz.${service}Service/Get`)
        return send(
          (isSession &&
            child &&
            items.find((s) =>
              [s.id, s.alias, s.runtimeId].some((key) =>
                Object.values(q.ref ?? {}).includes(key),
              ),
            )) ||
            value,
        );
      if (path === `/cxz.${service}Service/List`) return send({ items });
      if (path === `/cxz.${service}Service/Watch`)
        return frames([
          { items: items.map((value) => ({ id: value.id, value })) },
        ]);
    }
    if (path === "/cxz.SessionService/Transcript")
      return send({ events, metadata, snapshotSeq: "3" });
    if (path === "/cxz.SessionService/History") return send({ events: [] });
    if (
      [
        "/cxz.SessionService/Events",
        "/cxz.SessionService/Rows",
        "/cxz.SessionService/AuxEvents",
      ].includes(path)
    )
      return frames([]);
    if (path === "/cxz.SessionService/Stop") {
      state = "stopped";
      return send(session());
    }
    if (path === "/cxz.SessionService/SwitchAccount") {
      if (state !== "stopped" || q.account?.alias !== nextAccount.alias)
        return route.fulfill({
          status: 400,
          json: { code: "failed_precondition" },
        });
      account = nextAccount;
      return send(session());
    }
    if (path === "/cxz.SessionService/Resume") {
      if (resumeError) return route.fulfill({ status: 400, json: resumeError });
      if (!authenticated) return needsLogin();
      run = "resumed-run";
      state = "idle";
      return send(session());
    }
    if (path === "/cxz.SessionService/Fork") {
      if (forkError) return route.fulfill({ status: 400, json: forkError });
      child ??= {
        ...session(),
        id: id("child"),
        runtimeId: "child",
        alias: "forked-seal",
        clientId: q.clientId,
        status: { state: "stopped", runId: "", lastSeq: "3", pending: [] },
      };
      return send(child);
    }
    if (path === "/cxz.SessionService/Start" && q.ref?.runtimeId === "child") {
      if (!authenticated) return needsLogin(child.clientId);
      child.status = { ...child.status, state: "idle", runId: "child-run" };
      return send(child);
    }
    return send({});
  });
  await page.routeWebSocket("**/login/session?**", (socket) => {
    const entry = { socket, url: new URL(socket.url()), messages: [] };
    sockets.push(entry);
    socket.send(
      JSON.stringify({
        output: "Open https://claude.ai/oauth/authorize to log in.\n",
      }),
    );
    socket.onMessage((message) => entry.messages.push(message));
  });
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.goto("/");
  await page
    .getByRole("link", { name: /Claude session seal/ })
    .first()
    .click();
  return {
    requests,
    sockets,
    complete() {
      authenticated = true;
      sockets.at(-1).socket.send(JSON.stringify({ done: true }));
    },
    replaceRun() {
      run = "other-run";
    },
  };
}

test("expired Claude OAuth offers session login and resumes without replaying the prompt", async ({
  page,
}) => {
  const f = await fixture(page);
  await page.getByRole("button", { name: "Log in again", exact: true }).click();
  const panel = page.getByRole("region", { name: "Provider login" });
  await expect(panel).toBeVisible();
  expect(f.requests.filter((r) => r.path.endsWith("/Stop"))).toHaveLength(1);
  const target = f.sockets[0].url;
  expect(Object.fromEntries(target.searchParams)).toEqual({
    account: "work1",
    project: projectFixture().uuid,
    key: "original-creation",
  });
  await panel.getByLabel("Authentication code").fill("test-code");
  await panel.getByRole("button", { name: "Submit code" }).click();
  await expect.poll(() => f.sockets[0].messages).toEqual(["test-code\n"]);
  await expect(panel.getByLabel("Authentication code")).toHaveValue("");
  f.complete();
  await expect(panel).toBeHidden();
  await expect
    .poll(() => f.requests.filter((r) => r.path.endsWith("/Resume")).length)
    .toBe(1);
  expect(f.requests.find((r) => r.path.endsWith("/Resume")).q.runId).toBe(
    "original-run",
  );
  expect(f.requests.some((r) => r.path.endsWith("/Send"))).toBe(false);
});

test("Resume's structured credential failure opens Claude login automatically", async ({
  page,
}) => {
  const f = await fixture(page, { state: "stopped", authFailure: false });
  await expect(
    page.getByRole("button", { name: "Log in again", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Session menu" }).click();
  await page.getByRole("menuitem", { name: "Resume", exact: true }).click();
  const panel = page.getByRole("region", { name: "Provider login" });
  await expect(panel).toBeVisible();
  expect(f.requests.filter((r) => r.path.endsWith("/Stop"))).toHaveLength(0);
  f.complete();
  await expect
    .poll(() => f.requests.filter((r) => r.path.endsWith("/Resume")).length)
    .toBe(2);
  await expect(panel).toBeHidden();
});

async function changeAccount(page, confirmLabel) {
  await page.getByRole("button", { name: "Session menu" }).click();
  const change = page.getByRole("menuitem", { name: "Change account" });
  await expect(change.locator('[data-icon="switch-account"]')).toBeVisible();
  await change.click();
  const dialog = page.getByRole("dialog", { name: "Change account" });
  await dialog.getByRole("combobox", { name: "Account" }).click();
  await page.getByRole("option", { name: /Second Claude account/ }).click();
  await dialog.getByRole("button", { name: confirmLabel, exact: true }).click();
  await expect(dialog).toBeHidden();
}

function accountSwitchRequests(requests) {
  return requests
    .filter(({ path }) =>
      /^\/cxz\.SessionService\/(Stop|SwitchAccount|Start|Resume|Send)$/.test(
        path,
      ),
    )
    .map(({ path, q }) => ({ operation: path.split("/").at(-1), ...q }));
}

test("changing a running session's account resumes with the new account", async ({
  page,
}) => {
  const f = await fixture(page, { state: "working", authFailure: false });
  await changeAccount(page, "Stop and change");
  const panel = page.getByRole("region", { name: "Provider login" });
  await expect(panel).toBeVisible();
  expect(accountSwitchRequests(f.requests)).toMatchObject([
    { operation: "Stop", runId: "original-run" },
    { operation: "SwitchAccount", account: { alias: "work2" } },
    { operation: "Resume", runId: "original-run" },
  ]);
  expect(f.sockets[0].url.searchParams.get("account")).toBe("work2");
  f.complete();
  await expect(panel).toBeHidden();
  await expect
    .poll(() =>
      accountSwitchRequests(f.requests).map(({ operation }) => operation),
    )
    .toEqual(["Stop", "SwitchAccount", "Resume", "Resume"]);
});

test("changing a stopped session's account leaves it stopped", async ({
  page,
}) => {
  const f = await fixture(page, { state: "stopped", authFailure: false });
  await changeAccount(page, "Change");
  expect(accountSwitchRequests(f.requests)).toMatchObject([
    { operation: "SwitchAccount", account: { alias: "work2" } },
  ]);
});

test("a failed automatic start reports the account change separately", async ({
  page,
}) => {
  const f = await fixture(page, {
    authFailure: false,
    resumeError: { code: "unavailable", message: "runtime unreachable" },
  });
  await changeAccount(page, "Stop and change");
  await expect(
    page
      .getByRole("alert")
      .filter({
        hasText:
          "Account changed. Could not start the session. runtime unreachable",
      }),
  ).toBeVisible();
  expect(
    accountSwitchRequests(f.requests).map(({ operation }) => operation),
  ).toEqual(["Stop", "SwitchAccount", "Resume"]);
});

test("Fork saves the conversation before OAuth and only starts the child after login", async ({
  page,
}) => {
  const f = await fixture(page, { authFailure: false });
  const choose = async (name) => {
    await page.getByRole("button", { name: "Session menu" }).click();
    await page.getByRole("menuitem", { name, exact: true }).click();
  };
  await choose("Fork");
  await expect(page).toHaveURL(/\/c\/@local\/session\/forked-seal$/);
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    "Please continue",
  );
  const panel = page.getByRole("region", { name: "Provider login" });
  const forks = () => f.requests.filter((r) => r.path.endsWith("/Fork"));
  const starts = () => f.requests.filter((r) => r.path.endsWith("/Start"));
  expect(forks()).toHaveLength(1);
  expect(starts()).toHaveLength(0);
  expect(f.sockets).toHaveLength(0);
  const composer = page.getByRole("textbox", { name: "Message", exact: true });
  await composer.fill("Continue in the fork");
  const send = page.getByRole("button", { name: "Send", exact: true });
  await expect(send).toBeDisabled();
  await choose("Start");
  await expect(panel).toBeVisible();
  await expect.poll(() => f.sockets.length).toBe(1);
  expect(f.sockets[0].url.searchParams.get("account")).toBe("work1");
  expect(f.sockets[0].url.searchParams.get("project")).toBe(
    projectFixture().uuid,
  );
  expect(f.sockets[0].url.searchParams.get("key")).toBe(forks()[0].q.clientId);
  await panel.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(panel).toBeHidden();
  await expect(page).toHaveURL(/\/c\/@local\/session\/forked-seal$/);
  expect(starts()).toHaveLength(1);
  await expect(send).toBeDisabled();
  await choose("Start");
  await expect(panel).toBeVisible();
  await expect.poll(() => f.sockets.length).toBe(2);
  f.complete();
  await expect(panel).toBeHidden();
  await expect(send).toBeEnabled();
  expect(forks()).toHaveLength(1);
  expect(starts()).toHaveLength(3);
  for (const { q } of starts()) expect(q.ref).toEqual({ runtimeId: "child" });
  expect(f.requests.some((r) => r.path.endsWith("/Stop"))).toBe(false);
});

test("Fork reports the server's rejection in the conversation", async ({
  page,
}) => {
  await fixture(page, {
    authFailure: false,
    forkError: { code: "failed_precondition", message: "stale run_id" },
  });
  await page.getByRole("button", { name: "Session menu" }).click();
  await page.getByRole("menuitem", { name: "Fork", exact: true }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "stale run_id" }),
  ).toBeVisible();
  await expect(
    page.getByText(/check the session list before retrying/i),
  ).toHaveCount(0);
});

test("canceling or failing OAuth never resumes; login can be retried", async ({
  page,
}) => {
  const f = await fixture(page);
  await page.getByRole("button", { name: "Log in again", exact: true }).click();
  const panel = page.getByRole("region", { name: "Provider login" });
  await expect(panel).toBeVisible();
  f.sockets[0].socket.send(
    JSON.stringify({ error: "Provider login canceled" }),
  );
  await expectAPIError(page, "Provider login canceled");
  await panel.getByRole("button", { name: "Retry login" }).click();
  await expect.poll(() => f.sockets.length).toBe(2);
  await panel.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(panel).toBeHidden();
  expect(
    f.requests.some(
      (r) => r.path.endsWith("/Resume") || r.path.endsWith("/Send"),
    ),
  ).toBe(false);
});

test("a run replaced during OAuth is not resumed", async ({ page }) => {
  const f = await fixture(page);
  await page.getByRole("button", { name: "Log in again", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Provider login" }),
  ).toBeVisible();
  f.replaceRun();
  f.complete();
  await expect(
    page.getByRole("alert").filter({ hasText: "Session run changed" }),
  ).toBeVisible();
  expect(
    f.requests.some(
      (r) => r.path.endsWith("/Resume") || r.path.endsWith("/Send"),
    ),
  ).toBe(false);
});

test("Restart hands off its confirmation dialog to Claude login", async ({
  page,
}) => {
  const f = await fixture(page, { authFailure: false });
  await page.getByRole("button", { name: "Session menu" }).click();
  await page.getByRole("menuitem", { name: "Restart", exact: true }).click();
  const confirmation = page.getByRole("dialog", {
    name: "Restart session",
    exact: true,
  });
  await confirmation
    .getByRole("button", { name: "Restart", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Provider login" }),
  ).toBeVisible();
  await expect(confirmation).toBeHidden();
  await expect.poll(() => f.sockets.length).toBe(1);
  f.complete();
  await expect
    .poll(() => f.requests.filter((r) => r.path.endsWith("/Resume")).length)
    .toBe(2);
  expect(f.requests.filter((r) => r.path.endsWith("/Stop"))).toHaveLength(1);
});

test("Escape cancels login before the card's closing animation completes", async ({
  page,
}) => {
  const f = await fixture(page);
  await page.getByRole("button", { name: "Log in again", exact: true }).click();
  const panel = page.getByRole("region", { name: "Provider login" });
  await expect(panel).toBeVisible();
  await expect.poll(() => f.sockets.length).toBe(1);
  await page.keyboard.press("Escape");
  f.complete();
  await expect(panel).toBeHidden();
  expect(
    f.requests.some(
      (r) => r.path.endsWith("/Resume") || r.path.endsWith("/Send"),
    ),
  ).toBe(false);
});
