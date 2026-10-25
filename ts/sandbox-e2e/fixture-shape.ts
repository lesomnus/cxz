// What the sandbox seeds, named once. Several specs pin that every session of
// every project is listed, and that is worth pinning -- but the number is the
// fixture's, so adding a scenario should change this line and nothing else.
// internal/websandbox/server.go is where the scenarios live.
export const SANDBOX_SESSIONS = 9;

export const SANDBOX_PROJECT_IDS = {
  1: "36f69548-7a2c-86d8-a907-129a49306331",
  2: "7e91fd76-74cb-8da8-b007-369a4d91fe33",
} as const;
