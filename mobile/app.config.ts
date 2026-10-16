import fs from 'node:fs';
import path from 'node:path';
import type { ConfigContext, ExpoConfig } from 'expo/config';

/**
 * Extends app.json with the over-the-air update settings, which depend on
 * the environment a build or an update is made for.
 *
 * Updates are served by our own Go backend (GET /api/updates/manifest, see
 * backend/internal/adapters/web/ota_updates_handlers.go), never by EAS. The
 * manifest URL is OTA_UPDATES_URL when set, otherwise derived from an
 * absolute EXPO_PUBLIC_API_URL. With neither (plain local dev), updates are
 * disabled and the app always runs its embedded JS.
 *
 * runtimeVersion follows `version` (the "appVersion" policy): bump
 * `version` in app.json for every release that changes native code, so an
 * update is never delivered to a binary it isn't compatible with. JS-only
 * changes keep the same version and ship with `make ota-publish`.
 *
 * Code signing is enabled as soon as code-signing/certificate.pem exists
 * (created by `make ota-keys`): builds then refuse any update not signed
 * with the matching private key, which only the server holds.
 */
export default ({ config }: ConfigContext): ExpoConfig => {
  const apiUrl = process.env.EXPO_PUBLIC_API_URL;
  const updatesUrl =
    process.env.OTA_UPDATES_URL ??
    (apiUrl && /^https?:\/\//.test(apiUrl) ? `${apiUrl.replace(/\/+$/, '')}/updates/manifest` : undefined);

  const certificatePath = './code-signing/certificate.pem';
  const hasCertificate = fs.existsSync(path.join(__dirname, certificatePath));

  // App Store Connect needs a unique, increasing build number per upload of
  // the same version; scripts/ios-release.sh sets one from the clock.
  const iosBuildNumber = process.env.IOS_BUILD_NUMBER;

  return {
    ...(config as ExpoConfig),
    ios: { ...config.ios, ...(iosBuildNumber ? { buildNumber: iosBuildNumber } : {}) },
    runtimeVersion: { policy: 'appVersion' },
    updates: {
      ...config.updates,
      enabled: Boolean(updatesUrl),
      url: updatesUrl,
      // Check on launch, but never block startup waiting for the network:
      // a downloaded update applies on the next cold start.
      checkAutomatically: 'ON_LOAD',
      fallbackToCacheTimeout: 0,
      ...(hasCertificate
        ? {
            codeSigningCertificate: certificatePath,
            codeSigningMetadata: { keyid: 'main', alg: 'rsa-v1_5-sha256' },
          }
        : {}),
    },
  };
};
