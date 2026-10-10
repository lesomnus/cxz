import type { Preview } from "@storybook/react-vite";
import { useLayoutEffect } from "react";
import "@lesomnus/cxz-ui/styles.css";
import "./preview.css";
const preview: Preview = {
  initialGlobals: { theme: "dark" },
  globalTypes: {
    theme: {
      toolbar: {
        icon: "circlehollow",
        items: ["dark", "light"],
        dynamicTitle: true,
      },
    },
  },
  parameters: { layout: "centered" },
  decorators: [
    function Theme(Story, context) {
      useLayoutEffect(() => {
        document.documentElement.dataset.uiTheme = context.globals.theme;
      }, [context.globals.theme]);
      return (
        <div className="ui-showcase">
          <Story />
        </div>
      );
    },
  ],
};
export default preview;
