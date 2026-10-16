import type { Metadata } from "next";
import { LegalPage } from "@/components/LegalPage";
import { loadLegal } from "@/lib/legal";

const doc = loadLegal("privacy-policy.fr.md");

// Tant que le texte contient des [À COMPLÉTER], on ne laisse pas les moteurs l'indexer.
export const metadata: Metadata = {
  title: "Politique de confidentialité — Lagune",
  robots: doc.draft ? { index: false, follow: false } : undefined,
};

export default function Page() {
  return <LegalPage doc={doc} />;
}
