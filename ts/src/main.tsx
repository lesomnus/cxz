import { ThemeProvider } from "./shared/theme/theme";
import React from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/app";
import { WorkspaceView } from "./app/workspace-view";
import { LocaleProvider } from "./shared/i18n/i18n-react";
import { RouterProvider } from "@tanstack/react-router";
import { createWorkspaceRouter } from "./app/router";
const router = createWorkspaceRouter({ shell: App, view: WorkspaceView });
createRoot(document.getElementById("root")!).render(
  <ThemeProvider>
    <LocaleProvider>
      <RouterProvider router={router} />
    </LocaleProvider>
  </ThemeProvider>,
);
