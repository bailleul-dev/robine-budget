import type { MetadataRoute } from "next";
import { loadLegal } from "@/lib/legal";
import { site } from "@/lib/site";

export const dynamic = "force-static";

// Une page légale n'entre dans le sitemap que si elle est complète : tant qu'il reste
// des [À COMPLÉTER] elle est en noindex (voir lib/legal.ts).
export default function sitemap(): MetadataRoute.Sitemap {
  const legal = [
    { path: "/confidentialite/", file: "privacy-policy.fr.md" as const },
    { path: "/cgu/", file: "terms.fr.md" as const },
  ]
    .filter((p) => !loadLegal(p.file).draft)
    .map((p) => ({ url: `${site.url}${p.path}`, changeFrequency: "yearly" as const, priority: 0.3 }));
  return [{ url: site.url, changeFrequency: "monthly", priority: 1 }, ...legal];
}
