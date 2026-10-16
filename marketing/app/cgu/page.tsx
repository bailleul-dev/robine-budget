import type { Metadata } from "next";
import { LegalPage } from "@/components/LegalPage";
import { loadLegal } from "@/lib/legal";

const doc = loadLegal("terms.fr.md");

export const metadata: Metadata = {
  title: "Conditions d’utilisation — Lagune",
  robots: doc.draft ? { index: false, follow: false } : undefined,
};

export default function Page() {
  return <LegalPage doc={doc} />;
}
