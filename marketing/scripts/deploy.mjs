#!/usr/bin/env node
// Build 100 % local, puis dépôt du dossier out/ sur Vercel : rien n'est construit
// chez Vercel (le build lit ../docs/legal/, hors de ce projet).
//
//   npm run vercel:preview                  # aperçu (URL de preview)
//   npm run vercel:production             # production
//   node scripts/deploy.mjs --dry-run [--prod]   # tout sauf l'envoi
//
// Options : --allow-draft-legal (publier alors qu'il reste des [À COMPLÉTER]),
//           --allow-dirty (déployer avec des changements non commités).
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const out = path.join(root, "out");
const args = new Set(process.argv.slice(2));
const prod = args.has("--prod");
const dryRun = args.has("--dry-run");

const fail = (msg) => {
  console.error(`\n✖ ${msg}\n`);
  process.exit(1);
};
const step = (msg) => console.log(`\n▸ ${msg}`);
const run = (cmd, argv, opts = {}) =>
  spawnSync(cmd, argv, { cwd: root, stdio: "inherit", ...opts });

/** Lit .env.production comme le fait `next build`, pour valider les mêmes valeurs. */
function readEnvFile(file) {
  const env = {};
  if (!fs.existsSync(file)) return env;
  for (const line of fs.readFileSync(file, "utf8").split("\n")) {
    const m = line.match(/^\s*([A-Z0-9_]+)\s*=\s*(.*?)\s*$/);
    if (m) env[m[1]] = m[2].replace(/^["']|["']$/g, "");
  }
  return env;
}
const buildEnv = { ...readEnvFile(path.join(root, ".env.production")), ...process.env };

// ---------------------------------------------------------------- garde-fous
step(`Vérifications (${prod ? "PRODUCTION" : "aperçu"}${dryRun ? ", dry-run" : ""})`);

if (prod) {
  const siteUrl = buildEnv.NEXT_PUBLIC_SITE_URL;
  if (!siteUrl || !/^https:\/\//.test(siteUrl)) {
    fail("NEXT_PUBLIC_SITE_URL doit être défini en https:// pour la production (voir .env.production.example) : il fixe le sitemap et les métadonnées.");
  }
  if (!args.has("--allow-dirty")) {
    const dirty = spawnSync("git", ["status", "--porcelain", "--", ".", "../docs/legal"], { cwd: root, encoding: "utf8" });
    if (dirty.status === 0 && dirty.stdout.trim()) {
      fail(`Changements non commités (le site déployé ne correspondrait à aucun commit) :\n${dirty.stdout}\nCommitez, ou passez --allow-dirty.`);
    }
  }
}

const legalDir = path.join(root, "..", "docs", "legal");
const drafts = ["privacy-policy.fr.md", "terms.fr.md"].filter((f) =>
  fs.readFileSync(path.join(legalDir, f), "utf8").includes("[À COMPLÉTER"),
);
if (drafts.length) {
  const msg = `Textes légaux encore incomplets ([À COMPLÉTER]) : ${drafts.join(", ")}.`;
  if (prod && !args.has("--allow-draft-legal")) {
    fail(`${msg}\nLes pages seraient publiées en « version provisoire ». Complétez-les, ou passez --allow-draft-legal.`);
  }
  console.warn(`⚠ ${msg} Pages en noindex avec bandeau provisoire.`);
}

// --------------------------------------------------------------------- build
step("Build local (typecheck + next build → out/)");
fs.rmSync(out, { recursive: true, force: true });
fs.rmSync(path.join(root, ".next"), { recursive: true, force: true });
if (run("npm", ["run", "typecheck"]).status !== 0) fail("Le typecheck échoue.");
if (run("npm", ["run", "build"], { env: { ...process.env, NODE_ENV: "production" } }).status !== 0) fail("Le build échoue.");

// ----------------------------------------------------------- contrôle de out/
step("Contrôle du dossier out/");
for (const f of ["index.html", "404.html", "robots.txt", "sitemap.xml", "confidentialite/index.html", "cgu/index.html", "mark.svg"]) {
  if (!fs.existsSync(path.join(out, f))) fail(`out/${f} est absent : le build est incomplet.`);
}
const walk = (dir) =>
  fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(path.join(dir, e.name)) : [path.join(dir, e.name)]));
const files = walk(out);
const textFiles = files.filter((f) => /\.(html|txt|xml|js|css)$/.test(f));
// Une vraie URL de dev (http://localhost…, localhost:3100), pas le mot « localhost »
// que le runtime de Next contient dans son analyseur d'URL.
const devUrl = /https?:\/\/(localhost|127\.0\.0\.1)|\b(localhost|127\.0\.0\.1):\d{2,5}/;
const leaks = textFiles.filter((f) => devUrl.test(fs.readFileSync(f, "utf8")));
if (prod && leaks.length) {
  fail(`URL de développement (localhost) dans le build de production :\n${leaks.map((f) => "  " + path.relative(out, f)).join("\n")}`);
}
if (prod) {
  const sitemap = fs.readFileSync(path.join(out, "sitemap.xml"), "utf8");
  if (!sitemap.includes(buildEnv.NEXT_PUBLIC_SITE_URL)) fail("Le sitemap ne pointe pas sur NEXT_PUBLIC_SITE_URL : le build n'a pas pris l'URL de production.");
}
const maps = files.filter((f) => f.endsWith(".map"));
if (maps.length) console.warn(`⚠ ${maps.length} source map(s) dans out/ : elles seraient publiques.`);
const bytes = files.reduce((n, f) => n + fs.statSync(f).size, 0);
console.log(`  ${files.length} fichiers, ${(bytes / 1024 / 1024).toFixed(2)} Mo`);

// vercel.json accompagne le dossier : pas de build distant, en-têtes, slash final.
fs.copyFileSync(path.join(root, "vercel.json"), path.join(out, "vercel.json"));

// -------------------------------------------------------------------- envoi
const linkFile = path.join(root, ".vercel", "project.json");
const link = fs.existsSync(linkFile) ? JSON.parse(fs.readFileSync(linkFile, "utf8")) : {};
const orgId = process.env.VERCEL_ORG_ID ?? link.orgId;
const projectId = process.env.VERCEL_PROJECT_ID ?? link.projectId;
const deployArgs = ["deploy", out, "--yes", ...(prod ? ["--prod"] : [])];

if (dryRun) {
  step("Dry-run : rien n'est envoyé");
  console.log(`  Commande : vercel ${deployArgs.join(" ")}`);
  console.log(`  Projet lié : ${projectId ? `oui (${projectId})` : "NON (lancez `vercel link` dans marketing/)"}`);
  process.exit(0);
}
if (!orgId || !projectId) {
  fail("Aucun projet Vercel lié. Une seule fois : `cd marketing && vercel link` (ou définir VERCEL_ORG_ID et VERCEL_PROJECT_ID).");
}
const who = spawnSync("vercel", ["whoami"], { cwd: root, encoding: "utf8" });
if (who.status !== 0) fail("Vercel CLI absent ou non connecté : `npm i -g vercel` puis `vercel login`.");

step(`Envoi de out/ sur Vercel (${prod ? "production" : "aperçu"}) en tant que ${who.stdout.trim()}`);
// out/ n'a pas de .vercel : le lien passe par les variables d'environnement.
const res = run("vercel", deployArgs, { env: { ...process.env, VERCEL_ORG_ID: orgId, VERCEL_PROJECT_ID: projectId } });
if (res.status !== 0) fail("Le déploiement Vercel a échoué.");
console.log("\n✔ Terminé.");
