import { ThemeProvider } from "#src/shared/theme/theme.tsx";
import React from "react";
import { createRoot } from "react-dom/client";
import { App } from "#src/app/app.tsx";
import { WorkspaceView } from "#src/app/workspace-view.tsx";
import { LocaleProvider } from "#src/shared/i18n/i18n-react.tsx";
import { RouterProvider } from "@tanstack/react-router";
import { createWorkspaceRouter } from "#src/app/router.tsx";
const router = createWorkspaceRouter({ shell: App, view: WorkspaceView });
createRoot(document.getElementById("root")!).render(
  <ThemeProvider>
    <LocaleProvider>
      <RouterProvider router={router} />
    </LocaleProvider>
  </ThemeProvider>,
);
