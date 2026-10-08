import { ThemeProvider } from "./theme";
import React from "react";
import { createRoot } from "react-dom/client";
import { App, WorkspaceView } from "./app";
import { LocaleProvider } from "./i18n-react";
import { RouterProvider } from "@tanstack/react-router";
import { createWorkspaceRouter } from "./router";
const router = createWorkspaceRouter({ shell: App, view: WorkspaceView });
createRoot(document.getElementById("root")!).render(
  <ThemeProvider>
    <LocaleProvider>
      <RouterProvider router={router} />
    </LocaleProvider>
  </ThemeProvider>,
);
