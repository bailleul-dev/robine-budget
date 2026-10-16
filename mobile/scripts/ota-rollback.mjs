#!/usr/bin/env node
// Tells every app on the current runtime version to drop its downloaded
// updates and go back to the JS embedded in its binary. Publishing a new
// update afterwards supersedes the rollback. Usage — see `make ota-rollback`:
//
//   OTA_DEST=user@host:/srv/ota node scripts/ota-rollback.mjs
//   OTA_DEST=s3://budget-ota OTA_S3_ENDPOINT=… OTA_S3_ACCESS_KEY_ID=… OTA_S3_SECRET_ACCESS_KEY=… node scripts/ota-rollback.mjs
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { commit, parseDestination, publicExpoConfig, resolveRuntimeVersion } from './ota-lib.mjs';

const destination = parseDestination(process.env.OTA_DEST);
const runtimeVersion = process.env.OTA_RUNTIME_VERSION ?? resolveRuntimeVersion(publicExpoConfig(process.env));

const staged = fs.mkdtempSync(path.join(os.tmpdir(), 'ota-rollback-'));
fs.writeFileSync(path.join(staged, 'rollback'), '');

const name = String(Math.floor(Date.now() / 1000));
const published = await commit(destination, staged, runtimeVersion, name);
fs.rmSync(staged, { recursive: true, force: true });

console.log(`\n✔ Rollback to embedded published for runtime ${runtimeVersion} → ${published}\n`);
