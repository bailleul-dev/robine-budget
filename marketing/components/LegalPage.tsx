import { Footer } from "@/components/Footer";
import { Nav } from "@/components/Nav";
import type { LegalDoc } from "@/lib/legal";

export function LegalPage({ doc }: { doc: LegalDoc }) {
  return (
    <main>
      <div className="wrap">
        <Nav />
        <article className="legal">
          <h1>{doc.title}</h1>
          {doc.draft && (
            <p className="legal-draft" role="note">
              Version provisoire : certaines informations restent à compléter avant la publication.
            </p>
          )}
          <div dangerouslySetInnerHTML={{ __html: doc.html }} />
        </article>
      </div>
      <Footer />
    </main>
  );
}
