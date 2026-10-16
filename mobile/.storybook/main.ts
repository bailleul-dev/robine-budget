import type { StorybookConfig } from '@storybook/react-native-web-vite';

const config: StorybookConfig = {
  "stories": [
    "../src/**/*.mdx",
    "../src/**/*.stories.@(js|jsx|mjs|ts|tsx)"
  ],
  // Dropped the initializer's @chromatic-com/storybook (a hosted visual-
  // review service this project has no account for) and addon-vitest (pulls
  // in Playwright just to run story tests we're not writing yet) — kept
  // a11y and docs, both genuinely useful with zero external service needed.
  "addons": [
    "@storybook/addon-a11y",
    "@storybook/addon-docs"
  ],
  "framework": "@storybook/react-native-web-vite",
  // Resolves this app's own "@/..." → "src/..." alias (see tsconfig.json)
  // inside stories, the same way Metro/webpack already does for the app
  // itself — without this, `import { useTheme } from '@/hooks/use-theme'`
  // in a story fails to resolve under Storybook's separate Vite build.
  viteFinal: async (viteConfig) => {
    viteConfig.resolve ??= {};
    viteConfig.resolve.tsconfigPaths = true;
    return viteConfig;
  },
};
export default config;