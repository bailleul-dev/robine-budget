// Shared helpers for ota-publish.mjs / ota-rollback.mjs.
import { execFileSync, spawnSync } from 'node:child_process';
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

export const mobileDir = path.resolve(import.meta.dirname, '..');

export function fail(message) {
  console.error(`\n✖ ${message}\n`);
  process.exit(1);
}

export function run(cmd, args, options = {}) {
  const result = spawnSync(cmd, args, { stdio: 'inherit', cwd: mobileDir, ...options });
  if (result.status !== 0) fail(`${cmd} ${args.join(' ')} failed`);
}

/** The public Expo config, evaluated with `env` (so app.config.ts sees the
 * same EXPO_PUBLIC_* values the bundle is built with). */
export function publicExpoConfig(env) {
  const out = execFileSync('npx', ['expo', 'config', '--type', 'public', '--json'], {
    cwd: mobileDir,
    env,
    encoding: 'utf8',
  });
  return JSON.parse(out);
}

/** Resolves the runtime version exactly as the native build will. Only the
 * "appVersion" policy (see app.config.ts) or a literal string is supported:
 * the other policies can't be computed reliably outside a native build. */
export function resolveRuntimeVersion(config) {
  const rv = config.runtimeVersion;
  if (typeof rv === 'string') return rv;
  if (rv?.policy === 'appVersion') return config.version;
  fail(`unsupported runtimeVersion ${JSON.stringify(rv)} (use a string or { policy: 'appVersion' })`);
}

/** A destination is a local directory, an rsync/ssh target
 * ("user@host:/srv/ota") or an S3 bucket ("s3://bucket/optional/prefix",
 * e.g. Garage), matching the server's OTA_UPDATES_DIR or OTA_S3_BUCKET +
 * OTA_S3_PREFIX. S3 credentials come from OTA_S3_ENDPOINT,
 * OTA_S3_ACCESS_KEY_ID, OTA_S3_SECRET_ACCESS_KEY and OTA_S3_REGION (default
 * "garage"), and need write access to the bucket. */
export function parseDestination(dest) {
  if (!dest) fail('OTA_DEST is required (a local directory, user@host:/path or s3://bucket/prefix)');
  const s3 = /^s3:\/\/([^/]+)\/?(.*)$/.exec(dest);
  if (s3) {
    const { OTA_S3_ENDPOINT: endpoint, OTA_S3_ACCESS_KEY_ID: accessKeyId, OTA_S3_SECRET_ACCESS_KEY: secretAccessKey } = process.env;
    if (!endpoint || !/^https?:\/\//.test(endpoint) || !accessKeyId || !secretAccessKey) {
      fail('an s3:// OTA_DEST needs OTA_S3_ENDPOINT (http(s) URL), OTA_S3_ACCESS_KEY_ID and OTA_S3_SECRET_ACCESS_KEY');
    }
    const prefix = s3[2].replace(/^\/+|\/+$/g, '');
    return {
      s3: {
        endpoint: endpoint.replace(/\/+$/, ''),
        region: process.env.OTA_S3_REGION || 'garage',
        bucket: s3[1],
        accessKeyId,
        secretAccessKey,
      },
      dir: prefix,
    };
  }
  const remote = /^([^/:]+):(.+)$/.exec(dest);
  return remote ? { remote: true, host: remote[1], dir: remote[2] } : { remote: false, dir: path.resolve(dest) };
}

/** Moves a staged directory into its final, timestamped place. The server
 * only considers purely numeric directory names, so staging under a
 * ".staging-*" name and renaming at the end means it never sees a
 * half-copied update. S3 has no rename: there, the server ignores an update
 * until its metadata.json or rollback marker exists, so those go last. */
export async function commit(destination, stagedDir, runtimeVersion, name) {
  if (destination.s3) {
    const prefix = [destination.dir, runtimeVersion, name].filter(Boolean).join('/');
    const files = listFiles(stagedDir);
    const last = files.filter((f) => f === 'metadata.json' || f === 'rollback');
    const first = files.filter((f) => !last.includes(f));
    await uploadAll(destination.s3, stagedDir, prefix, first);
    await uploadAll(destination.s3, stagedDir, prefix, last);
    return `s3://${destination.s3.bucket}/${prefix}`;
  }
  const finalDir = `${destination.dir}/${runtimeVersion}/${name}`;
  const stagingDir = `${destination.dir}/${runtimeVersion}/.staging-${name}`;
  if (destination.remote) {
    run('ssh', [destination.host, `mkdir -p '${destination.dir}/${runtimeVersion}'`]);
    run('rsync', ['-a', '--delete', `${stagedDir}/`, `${destination.host}:${stagingDir}/`]);
    run('ssh', [destination.host, `mv '${stagingDir}' '${finalDir}'`]);
  } else {
    fs.mkdirSync(path.dirname(stagingDir), { recursive: true });
    fs.rmSync(stagingDir, { recursive: true, force: true });
    fs.cpSync(stagedDir, stagingDir, { recursive: true });
    fs.renameSync(stagingDir, finalDir);
  }
  return destination.remote ? `${destination.host}:${finalDir}` : finalDir;
}

/** Relative, slash-separated paths of every file under dir. */
function listFiles(dir) {
  return fs
    .readdirSync(dir, { recursive: true, withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) => path.relative(dir, path.join(entry.parentPath, entry.name)).split(path.sep).join('/'));
}

async function uploadAll(s3, stagedDir, prefix, files, concurrency = 8) {
  const queue = [...files];
  const worker = async () => {
    for (let file; (file = queue.shift()) !== undefined; ) {
      await putObject(s3, `${prefix}/${file}`, fs.readFileSync(path.join(stagedDir, file)));
    }
  };
  await Promise.all(Array.from({ length: Math.min(concurrency, files.length) }, worker));
}

/** PUT one object, path-style (the only addressing Garage supports), signed
 * with AWS Signature Version 4. */
async function putObject(s3, key, body) {
  const url = new URL(s3.endpoint);
  const canonicalUri = `${url.pathname.replace(/\/+$/, '')}/${s3.bucket}/${key}`
    .split('/')
    .map((segment) => encodeURIComponent(segment).replace(/[!'()*]/g, (c) => `%${c.charCodeAt(0).toString(16).toUpperCase()}`))
    .join('/');
  const amzDate = new Date().toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
  const date = amzDate.slice(0, 8);
  const payloadHash = sha256Hex(body);
  const headers = { host: url.host, 'x-amz-content-sha256': payloadHash, 'x-amz-date': amzDate };
  const signedHeaders = Object.keys(headers).join(';');
  const canonicalRequest = [
    'PUT',
    canonicalUri,
    '',
    ...Object.entries(headers).map(([k, v]) => `${k}:${v}`),
    '',
    signedHeaders,
    payloadHash,
  ].join('\n');
  const scope = `${date}/${s3.region}/s3/aws4_request`;
  const stringToSign = ['AWS4-HMAC-SHA256', amzDate, scope, sha256Hex(canonicalRequest)].join('\n');
  let signingKey = `AWS4${s3.secretAccessKey}`;
  for (const part of [date, s3.region, 's3', 'aws4_request']) signingKey = hmac(signingKey, part);
  const signature = hmac(signingKey, stringToSign).toString('hex');

  const response = await fetch(`${url.origin}${canonicalUri}`, {
    method: 'PUT',
    headers: {
      'x-amz-content-sha256': payloadHash,
      'x-amz-date': amzDate,
      authorization: `AWS4-HMAC-SHA256 Credential=${s3.accessKeyId}/${scope}, SignedHeaders=${signedHeaders}, Signature=${signature}`,
    },
    body,
  });
  if (!response.ok) fail(`upload of s3://${s3.bucket}/${key} failed: ${response.status} ${await response.text()}`);
}

const sha256Hex = (data) => crypto.createHash('sha256').update(data).digest('hex');
const hmac = (key, data) => crypto.createHmac('sha256', key).update(data).digest();
