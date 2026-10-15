import { mergeConfig } from "vite";
import { noxConfig } from "../scripts/nox-vite.mjs";
import type { StorybookConfig } from "@storybook/react-vite";
const config: StorybookConfig = {
  stories: ["../packages/nox/src/**/*.stories.tsx"],
  viteFinal: (config) => mergeConfig(config, noxConfig("serve")),
  addons: ["@storybook/addon-docs"],
  framework: "@storybook/react-vite",
  core: { disableTelemetry: true },
};
export default config;
