import { useLayoutEffect } from "react";
import type { Preview } from "@storybook/react-vite";
import { localeStore } from "../src/i18n";
import "../src/style.css";

const preview: Preview = {
  globalTypes: {
    theme: {
      description: "Color theme",
      toolbar: {
        icon: "circlehollow",
        items: ["dark", "light"],
        dynamicTitle: true,
      },
    },
    locale: {
      description: "Language",
      toolbar: { icon: "globe", items: ["ko", "en"], dynamicTitle: true },
    },
  },
  initialGlobals: { theme: "dark", locale: "ko" },
  parameters: {
    layout: "centered",
    controls: { matchers: { color: /(background|color)$/i, date: /Date$/i } },
  },
  decorators: [
    function Appearance(Story, context) {
      useLayoutEffect(() => {
        document.documentElement.dataset.theme = context.globals.theme;
        document.documentElement.lang = context.globals.locale;
        void localeStore.activate(context.globals.locale);
      }, [context.globals.theme, context.globals.locale]);
      return <Story />;
    },
  ],
};

export default preview;
