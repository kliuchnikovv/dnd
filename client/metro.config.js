// Learn more https://docs.expo.dev/guides/monorepos/ and https://docs.expo.dev/versions/v57.0.0/
const { getDefaultConfig } = require('expo/metro-config');
const path = require('path');

const config = getDefaultConfig(__dirname);

// Aliases so vendored @genie/ds and donor components resolve to their local copies.
config.resolver.alias = {
  ...(config.resolver.alias ?? {}),
  '@genie/ds': path.resolve(__dirname, 'src/ds'),
  '@genie/front/themes': path.resolve(__dirname, 'src/theme'),
};

module.exports = config;
