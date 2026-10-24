import { mergeConfig } from "vite";
import { noxConfig } from "../scripts/nox-vite.mjs";
import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
  stories: [
    "../src/**/*.stories.@(ts|tsx)",
    "../packages/nox/src/components/action-menu/*.stories.tsx",
    "../packages/nox/src/components/panel-item/*.stories.tsx",
    "../packages/nox/src/components/page-toolbar/*.stories.tsx",
    "../packages/nox/src/components/icon-value-menu/*.stories.tsx",
    "../packages/nox/src/components/card/*.stories.tsx",
    "../packages/nox/src/components/compact-number/*.stories.tsx",
    "../packages/nox/src/components/duration-display/*.stories.tsx",
    "../packages/nox/src/components/suggestion-grid/*.stories.tsx",
    "../packages/nox/src/components/split-layout/*.stories.tsx",
    "../packages/nox/src/components/resize-handle/*.stories.tsx",
    "../packages/nox/src/components/audio-waveform/*.stories.tsx",
    "../packages/nox/src/components/badged-icon/*.stories.tsx",
    "../packages/nox/src/components/overflow-hint/*.stories.tsx",
    "../packages/nox/src/terminal/*.stories.tsx",
  ],
  viteFinal: (config) => mergeConfig(config, noxConfig("serve")),
  addons: ["@storybook/addon-docs"],
  framework: "@storybook/react-vite",
  core: { disableTelemetry: true },
};

export default config;
