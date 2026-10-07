import React from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app";
import { LocaleProvider } from "./i18n-react";
createRoot(document.getElementById("root")!).render(
  <LocaleProvider>
    <App />
  </LocaleProvider>,
);
