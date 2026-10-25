import { ThemeProvider } from "#src/shared/theme/theme.tsx";
import React from "react";
import { createRoot } from "react-dom/client";
import { App } from "#src/app/app.tsx";
import { WorkspaceView } from "#src/app/workspace-view.tsx";
import { LocaleProvider } from "#src/shared/i18n/i18n-react.tsx";
import { RouterProvider } from "@tanstack/react-router";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { installReactPerformanceBudget } from "#src/shared/browser/react-performance.ts";
import {
  installClientLogs,
  recordClientError,
} from "#src/shared/logs/client-logs.ts";
const disposeLogs = installClientLogs();
import.meta.hot?.dispose(disposeLogs);
if (import.meta.env.DEV) {
  const dispose = installReactPerformanceBudget();
  import.meta.hot?.dispose(dispose);
}
const router = createWorkspaceRouter({ shell: App, view: WorkspaceView });
createRoot(document.getElementById("root")!, {
  onUncaughtError: (error) => recordClientError(error, "Uncaught React error"),
  onCaughtError: (error) => recordClientError(error, "Caught React error"),
  onRecoverableError: (error) =>
    recordClientError(error, "Recoverable React error"),
}).render(
  <ThemeProvider>
    <LocaleProvider>
      <RouterProvider router={router} />
    </LocaleProvider>
  </ThemeProvider>,
);
