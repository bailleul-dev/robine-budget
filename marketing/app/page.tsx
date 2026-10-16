import Image from "next/image";
import { Compare } from "@/components/Compare";
import { DownloadButton } from "@/components/DownloadButton";
import { Footer } from "@/components/Footer";
import { Nav } from "@/components/Nav";
import { Wave } from "@/components/Wave";

const features = [
  {
    icon: "◔",
    title: "Tous vos comptes, un seul endroit",
    text: "Importez vos relevés CSV, quelle que soit la banque : Lagune retient le format et range chaque opération à sa place.",
  },
  {
    icon: "✦",
    title: "Des catégories qui s’apprennent",
    text: "Lagune classe vos dépenses toute seule, et s’améliore à chaque fois que vous corrigez une catégorie.",
  },
  {
    icon: "☾",
    title: "Le futur en lilas",
    text: "Loyers, abonnements, salaire : Lagune repère ce qui revient et le place sur votre courbe, pour voir où vous en serez.",
  },
  {
    icon: "↻",
    title: "Les récurrents, détectés",
    text: "Plus besoin de tout saisir : les opérations qui reviennent chaque mois vous sont proposées, il ne reste qu’à valider.",
  },
  {
    icon: "♡",
    title: "Un budget à deux",
    text: "Invitez un proche avec un lien nominatif. Il voit et annote vos comptes, et vous retirez son accès quand vous voulez.",
  },
  {
    icon: "✧",
    title: "Demandez à Claude",
    text: "Connectez Lagune à Claude pour poser vos questions en langage naturel : « Combien ai-je dépensé en restaurants ? »",
  },
];

const steps = [
  { n: "1", title: "Importez", text: "Déposez le relevé CSV de votre banque. Un format enregistré, c’est fait pour les prochaines fois." },
  { n: "2", title: "Lagune range", text: "Chaque opération trouve sa catégorie, les récurrents sont repérés et proposés." },
  { n: "3", title: "Anticipez", text: "Votre courbe se prolonge en lilas : vous voyez le mois à venir avant qu’il n’arrive." },
];

const trust = [
  { title: "Vos données restent les vôtres", text: "Aucune publicité, aucune revente. Lagune n’en a pas besoin : elle ne fait que ce pour quoi vous l’utilisez." },
  { title: "Export et suppression en un geste", text: "Récupérez l’intégralité de vos données au format JSON, ou supprimez votre compte depuis l’app : tout est effacé." },
  { title: "Un accès protégé", text: "Mots de passe protégés par un hachage robuste, connexion chiffrée, et possibilité de vous déconnecter de tous vos appareils." },
  { title: "Vous décidez du partage", text: "Seules les personnes que vous invitez voient vos comptes, avec une adresse e-mail vérifiée, et vous pouvez retirer l’accès à tout moment." },
];

const faq = [
  {
    q: "Lagune se connecte-t-elle à ma banque ?",
    a: "Pas encore : aujourd’hui, vous importez vos relevés vous-même, en CSV, et Lagune ne vous demande pas vos identifiants bancaires. Une synchronisation automatique est en développement.",
  },
  {
    q: "Qui peut voir mes comptes ?",
    a: "Vous, et uniquement les personnes que vous invitez. Chaque invitation est nominative et révocable à tout moment.",
  },
  {
    q: "Mes données sont-elles revendues ?",
    a: "Jamais. Il n’y a pas de publicité dans Lagune et vos données ne sont ni vendues ni cédées.",
  },
  {
    q: "Puis-je tout supprimer ?",
    a: "Oui, depuis les réglages de l’app. La suppression efface votre compte et l’ensemble de vos données. Vous pouvez aussi en demander un export complet avant.",
  },
  {
    q: "Lagune me donne-t-elle des conseils financiers ?",
    a: "Non. Les catégories et les prévisions sont des repères indicatifs pour y voir plus clair, pas des conseils.",
  },
  {
    q: "Sur quoi puis-je l’utiliser ?",
    a: "Lagune arrive d’abord sur iPhone. Le bouton de téléchargement s’activera dès la publication.",
  },
];

export default function Home() {
  return (
    <main>
      <section className="hero">
        <div className="aurora" aria-hidden="true"><i /><i /><i /></div>
        <div className="wrap">
          <Nav />
          <div className="hero-grid">
            <div>
              <span className="pill">Budget personnel, en douceur</span>
              <h1>Votre argent, <em>calme comme l’eau.</em></h1>
              <p className="lead">
                Lagune rassemble vos comptes, range vos dépenses toute seule et vous montre où vous en serez
                dans un mois. Sans alarme rouge, sans tableur.
              </p>
              <div className="cta">
                <DownloadButton />
                <a className="btn ghost" href="#fonctionnalites">Découvrir</a>
              </div>
              <div className="reassure">
                <span>Export et suppression en un geste</span>
                <span>Aucune publicité</span>
                <span>Pas de conseil, juste de la clarté</span>
              </div>
            </div>

            <div className="device" aria-hidden="true">
              <div className="screen">
                <h3>Compte courant</h3>
                <div className="balance">2 480,36 €</div>
                <span className="delta">+ 312 € ce mois-ci</span>
                <div className="wave"><Wave /></div>
                <div className="card">
                  <div className="row"><div><b>Supermarché</b><small>Aujourd’hui</small></div><span className="amt out">− 64,20 €</span></div>
                  <div className="row"><div><b>Loyer</b><small>Le 5 du mois</small></div><span className="tag">Prévu</span></div>
                  <div className="row"><div><b>Salaire</b><small>Le 28</small></div><span className="amt" style={{ color: "var(--positive)" }}>+ 2 350 €</span></div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="section" id="fonctionnalites">
        <div className="wrap">
          <header className="section-head">
            <span className="pill">Fonctionnalités</span>
            <h2>Un budget qui coule de source</h2>
            <p>Tout ce qu’il faut pour y voir clair, rien de ce qui stresse.</p>
          </header>
          <div className="feature-grid">
            {features.map((f) => (
              <article className="feature" key={f.title}>
                <span className="feature-icon" aria-hidden="true">{f.icon}</span>
                <h3>{f.title}</h3>
                <p>{f.text}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section className="section">
        <div className="wrap">
          <header className="section-head">
            <span className="pill">Comment ça marche</span>
            <h2>Trois gestes, puis Lagune s’occupe du reste</h2>
          </header>
          <ol className="steps">
            {steps.map((s) => (
              <li className="step" key={s.n}>
                <span className="step-n">{s.n}</span>
                <h3>{s.title}</h3>
                <p>{s.text}</p>
              </li>
            ))}
          </ol>
        </div>
      </section>

      <section className="section forecast">
        <div className="aurora soft" aria-hidden="true"><i /><i /><i /></div>
        <div className="wrap forecast-grid">
          <div>
            <span className="pill lilac">Prévisions</span>
            <h2>Voyez le mois à venir avant qu’il n’arrive</h2>
            <p className="lead">
              Ce qui est passé est en teal. Ce qui vous attend — loyer, abonnements, salaire — est en lilas. Le futur
              n’est plus une source d’inquiétude, c’est une ligne qui se prolonge.
            </p>
          </div>
          <div className="forecast-card">
            <div className="row"><b>Solde prévu au 30</b><span className="amt">2 118 €</span></div>
            <div className="wave big"><Wave split={0.5} marker={false} /></div>
            <div className="legend"><span className="dot teal" /> Réalisé <span className="dot violet" /> Prévu</div>
          </div>
        </div>
      </section>

      <section className="section" id="comparatif">
        <div className="wrap">
          <header className="section-head">
            <span className="pill">Comparatif</span>
            <h2>Ce que Lagune fait autrement</h2>
            <p>
              Honnêtement : la synchronisation automatique de la banque arrive, elle n’est pas encore là. En
              attendant, vous gardez la main sur ce que vous importez.
            </p>
          </header>
          <Compare />
          <p className="compare-note">
            Comparaison générale par type d’approche, sans viser une offre en particulier. « Variable » signifie que
            cela dépend des offres.
          </p>
        </div>
      </section>

      <section className="section" id="confiance">
        <div className="wrap">
          <header className="section-head">
            <span className="pill">Confiance</span>
            <h2>Votre argent est une affaire privée</h2>
            <p>Lagune est conçue pour en savoir le moins possible, et vous laisser le contrôle.</p>
          </header>
          <div className="trust-grid">
            {trust.map((t) => (
              <article className="trust" key={t.title}>
                <h3>{t.title}</h3>
                <p>{t.text}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section className="section" id="faq">
        <div className="wrap narrow">
          <header className="section-head">
            <span className="pill">Questions</span>
            <h2>On vous répond</h2>
          </header>
          <div className="faq">
            {faq.map((item) => (
              <details key={item.q}>
                <summary>{item.q}</summary>
                <p>{item.a}</p>
              </details>
            ))}
          </div>
        </div>
      </section>

      <section className="section final">
        <div className="wrap">
          <div className="final-card">
            <div className="aurora soft" aria-hidden="true"><i /><i /><i /></div>
            <div className="final-inner">
              <Image src="/mark.svg" alt="" width={64} height={64} className="final-mark" />
              <h2>Respirez, on s’occupe des chiffres.</h2>
              <p>Un budget calme, qui vous laisse le contrôle.</p>
              <div className="cta center"><DownloadButton /></div>
            </div>
          </div>
        </div>
      </section>

      <Footer />
    </main>
  );
}
