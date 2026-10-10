import { UIProvider } from "@lesomnus/cxz-ui";
import { useLayoutEffect } from "react";
import type { Preview } from "@storybook/react-vite";
import { t, localeStore } from "../src/i18n";
import { useLocale } from "../src/i18n-react";
import { settingsStore } from "../src/settings";
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
      toolbar: { icon: "globe", items: ["en", "ko"], dynamicTitle: true },
    },
  },
  initialGlobals: { theme: "dark", locale: "en" },
  parameters: {
    layout: "centered",
    controls: { matchers: { color: /(background|color)$/i, date: /Date$/i } },
  },
  decorators: [
    function Appearance(Story, context) {
      const { locale } = useLocale();
      useLayoutEffect(() => {
        // Apply the toolbar theme to editor hooks too, without writing settings.json.
        const store = settingsStore();
        store.sync(
          JSON.stringify({
            ...store.snapshot().document,
            "ui.theme": context.globals.theme,
          }),
        );
        document.documentElement.dataset.theme = context.globals.theme;
        document.documentElement.lang = context.globals.locale;
        void localeStore.activate(context.globals.locale);
      }, [context.globals.theme, context.globals.locale]);
      return (
        <UIProvider translate={t} locale={locale}>
          <Story />
        </UIProvider>
      );
    },
  ],
};

export default preview;
