import fs from "node:fs";
import path from "node:path";
import { marked } from "marked";

/** Les textes légaux vivent dans docs/legal/ (source unique, relue par un
 * juriste) : le site les rend tels quels au build, il ne les duplique pas. */
const LEGAL_DIR = path.join(process.cwd(), "..", "docs", "legal");

export type LegalDoc = { html: string; title: string; draft: boolean };

export function loadLegal(file: "privacy-policy.fr.md" | "terms.fr.md"): LegalDoc {
  const raw = fs.readFileSync(path.join(LEGAL_DIR, file), "utf8");
  // Le brouillon commence par un titre H1 puis un bandeau « > Brouillon… » : on
  // sort le titre pour la page, et on garde le bandeau seulement s'il reste
  // des champs à compléter.
  const title = raw.match(/^#\s+(.+)$/m)?.[1] ?? "Document légal";
  const body = raw.replace(/^#\s+.+\n+/, "").replace(/^>.*\n+/, "");
  const draft = raw.includes("[À COMPLÉTER");
  return { html: marked.parse(body, { async: false }) as string, title, draft };
}
