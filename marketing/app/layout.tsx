import type { Metadata } from "next";
import { Manrope } from "next/font/google";
import { site } from "@/lib/site";
import "./globals.css";

const manrope = Manrope({ subsets: ["latin"], weight: ["400", "500", "600", "700", "800"], display: "swap" });

export const metadata: Metadata = {
  metadataBase: new URL(site.url),
  title: "Lagune — Votre argent, tout en douceur",
  description:
    "Lagune rassemble vos comptes, catégorise vos dépenses et prévoit votre solde. Un budget calme, sans alarme rouge.",
  icons: { icon: "/mark.svg" },
  openGraph: {
    title: "Lagune — Votre argent, tout en douceur",
    description: "Un budget calme : vos comptes, vos catégories, vos prévisions.",
    type: "website",
    locale: "fr_FR",
    siteName: "Lagune",
  },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="fr" className={manrope.className}>
      <body>{children}</body>
    </html>
  );
}
