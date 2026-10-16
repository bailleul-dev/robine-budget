import type { NextConfig } from "next";

// Export statique : du HTML/CSS/JS, aucun serveur Node, aucune fonction
// serverless — le coût d'hébergement reste fixe quel que soit le trafic.
const config: NextConfig = {
  output: "export",
  reactStrictMode: true,
  poweredByHeader: false,
  images: { unoptimized: true },
  trailingSlash: true,
};

export default config;
