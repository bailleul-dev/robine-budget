/** Réglages du site. Renseigner appStoreUrl quand l'app est publiée : les
 * boutons « Télécharger » basculent alors sur le lien, sinon ils annoncent
 * « Bientôt disponible » (on n'affiche jamais un lien mort). */
export const site = {
  name: "Lagune",
  url: process.env.NEXT_PUBLIC_SITE_URL ?? "https://lagune.bailleul.dev",
  appStoreUrl: process.env.NEXT_PUBLIC_APP_STORE_URL ?? null,
  contactEmail: process.env.NEXT_PUBLIC_CONTACT_EMAIL ?? null,
};
