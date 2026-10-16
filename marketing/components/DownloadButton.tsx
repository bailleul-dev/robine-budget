import { site } from "@/lib/site";

/** Lien App Store si l'app est publiée, sinon un état honnête « bientôt ». */
export function DownloadButton({ ghost = false }: { ghost?: boolean }) {
  if (site.appStoreUrl) {
    return (
      <a className={ghost ? "btn ghost" : "btn"} href={site.appStoreUrl}>
        Télécharger l’app
      </a>
    );
  }
  return (
    <span className={ghost ? "btn ghost soon" : "btn soon"} aria-disabled="true">
      Bientôt sur iPhone
    </span>
  );
}
