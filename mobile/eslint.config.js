// https://docs.expo.dev/guides/using-eslint/
const { defineConfig } = require('eslint/config');
const expoConfig = require("eslint-config-expo/flat");
// For more info, see https://github.com/storybookjs/eslint-plugin-storybook#configuration-flat-config-format
const storybook = require("eslint-plugin-storybook");

module.exports = defineConfig([
  expoConfig,
  ...storybook.configs["flat/recommended"],
  {
    ignores: ["dist/*"],
  }
]);
