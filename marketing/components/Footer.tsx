import Image from "next/image";
import Link from "next/link";
import { site } from "@/lib/site";

export function Footer() {
  return (
    <footer className="footer">
      <div className="wrap footer-grid">
        <div>
          <span className="brand">
            <Image src="/mark.svg" alt="" width={34} height={34} />
            lagune
          </span>
          <p className="footer-note">Votre argent, tout en douceur.</p>
        </div>
        <nav aria-label="Liens légaux">
          <Link href="/confidentialite">Confidentialité</Link>
          <Link href="/cgu">Conditions d’utilisation</Link>
          {site.contactEmail && <a href={`mailto:${site.contactEmail}`}>Contact</a>}
        </nav>
      </div>
      <div className="wrap footer-legal">© {new Date().getFullYear()} Lagune. Lagune ne fournit pas de conseil financier.</div>
    </footer>
  );
}
