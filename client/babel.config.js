module.exports = function (api) {
  api.cache(true);
  return {
    presets: ['babel-preset-expo'],
    // reanimated 4: worklets plugin must be listed last.
    plugins: ['react-native-worklets/plugin'],
  };
};
