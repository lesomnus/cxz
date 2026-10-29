import { mergeConfig } from "vite";
import { noxConfig } from "../scripts/nox-vite.mjs";
import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
  // Nox's standalone catalog belongs to .storybook-ui. This catalog only
  // exercises cxz components and their product-specific integrations.
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  viteFinal: (config) => mergeConfig(config, noxConfig("serve")),
  addons: ["@storybook/addon-docs"],
  framework: "@storybook/react-vite",
  core: { disableTelemetry: true },
};

export default config;
