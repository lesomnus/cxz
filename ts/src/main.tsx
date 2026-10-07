import { ThemeProvider } from "./theme";
import React from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app";
import { LocaleProvider } from "./i18n-react";
createRoot(document.getElementById("root")!).render(
  <ThemeProvider>
    <LocaleProvider>
      <App />
    </LocaleProvider>
  </ThemeProvider>,
);
