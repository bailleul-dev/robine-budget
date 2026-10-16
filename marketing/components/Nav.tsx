import Image from "next/image";
import Link from "next/link";

/** Navigation : ancres de la page d'accueil, utilisable depuis les autres pages. */
export function Nav() {
  return (
    <nav className="nav" aria-label="Principal">
      <Link className="brand" href="/">
        <Image src="/mark.svg" alt="" width={38} height={38} priority />
        lagune
      </Link>
      <div className="nav-links">
        <Link href="/#fonctionnalites">Fonctionnalités</Link>
        <Link href="/#comparatif">Comparatif</Link>
        <Link href="/#confiance">Confiance</Link>
        <Link href="/#faq">Questions</Link>
      </div>
    </nav>
  );
}
