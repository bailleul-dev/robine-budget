#!/usr/bin/env node
// Publishes the current JS as an over-the-air update to our own server
// (never EAS). Usage — see `make ota-publish`:
//
//   OTA_API_URL=https://budget.example.com/api OTA_DEST=user@host:/srv/ota node scripts/ota-publish.mjs
//   OTA_API_URL=… OTA_DEST=s3://budget-ota OTA_S3_ENDPOINT=… OTA_S3_ACCESS_KEY_ID=… OTA_S3_SECRET_ACCESS_KEY=… node scripts/ota-publish.mjs
//
// OTA_API_URL is baked into the bundle as EXPO_PUBLIC_API_URL, exactly like
// a store build, so it must match the binaries' own API URL.
import fs from 'node:fs';
import path from 'node:path';
import { commit, fail, mobileDir, parseDestination, publicExpoConfig, resolveRuntimeVersion, run } from './ota-lib.mjs';

const apiUrl = process.env.OTA_API_URL;
if (!apiUrl || !/^https?:\/\//.test(apiUrl)) {
  fail('OTA_API_URL must be the absolute API URL the app talks to, e.g. https://budget.example.com/api');
}
const destination = parseDestination(process.env.OTA_DEST);
const env = { ...process.env, EXPO_PUBLIC_API_URL: apiUrl, NODE_ENV: 'production' };

const config = publicExpoConfig(env);
const runtimeVersion = resolveRuntimeVersion(config);
if (!config.updates?.enabled) fail('updates are disabled in the resolved app config (see app.config.ts)');

const outDir = path.join(mobileDir, '.ota-dist');
fs.rmSync(outDir, { recursive: true, force: true });

console.log(`\n→ Exporting iOS + Android bundles for runtime ${runtimeVersion}…\n`);
run('npx', ['expo', 'export', '--platform', 'ios', '--platform', 'android', '--output-dir', outDir], { env });

// Web-only leftovers copied from public/ aren't part of a native update.
for (const entry of fs.readdirSync(outDir)) {
  if (!['_expo', 'assets', 'metadata.json'].includes(entry)) fs.rmSync(path.join(outDir, entry), { recursive: true });
}
fs.writeFileSync(path.join(outDir, 'expoConfig.json'), JSON.stringify(config));

const name = String(Math.floor(Date.now() / 1000));
const published = await commit(destination, outDir, runtimeVersion, name);
fs.rmSync(outDir, { recursive: true, force: true });

console.log(`\n✔ Update ${name} published for runtime ${runtimeVersion} → ${published}`);
console.log('  Installed apps pick it up on their next launch, and run it on the one after.\n');
