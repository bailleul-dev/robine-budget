# Lagune — site marketing

Landing page + pages légales, en **export statique** Next.js (`output: "export"`) :
du HTML/CSS/JS, aucun serveur Node, aucune fonction serverless — le coût
d'hébergement ne dépend pas du trafic.

```sh
npm install
npm run dev        # http://localhost:3100
npm run build      # génère out/
npm run typecheck
```

## Identité

Mêmes tokens que l'app (`../mobile/src/constants/theme.ts`), recopiés en variables
CSS dans `app/globals.css` (clair + sombre automatiques) : teal = marque, lilas =
le futur, corail pour les dépenses (jamais de rouge d'alarme), Manrope, grands
rayons, aurores floues et cartes en verre. Si l'identité de l'app change,
mettre à jour ce fichier dans le même changement.

## Contenu

- `components/Compare.tsx` : tableau comparatif par *type d’approche* (jamais par marque).
  Cases vraies en général, « Variable » = dépend des offres ; la synchronisation
  bancaire y est « En développement ». À passer à « Oui » à la mise en service.
- `app/page.tsx` : toutes les sections de la landing (textes en tête de fichier).
  Ne promettre que ce que l'app fait réellement : pas de synchro bancaire (fermée
  au public), pas de tarif, pas de « hébergé en France » tant que l'hébergeur
  n'est pas choisi.
- `app/confidentialite`, `app/cgu` : rendent `../docs/legal/*.md` **au build**
  (source unique). Tant qu'il reste des `[À COMPLÉTER]`, la page affiche un
  bandeau « version provisoire » et est en `noindex`, hors sitemap.
- `lib/site.ts` : URL du site et lien App Store. Sans lien, les boutons
  affichent « Bientôt sur iPhone » (jamais de lien mort).

## Variables d'environnement (facultatives)

| Variable | Rôle |
|---|---|
| `NEXT_PUBLIC_SITE_URL` | URL publique (sitemap, métadonnées). Défaut `https://lagune.bailleul.dev` |
| `NEXT_PUBLIC_APP_STORE_URL` | Active les boutons « Télécharger l'app » |
| `NEXT_PUBLIC_CONTACT_EMAIL` | Affiche « Contact » dans le pied de page |

## Déploiement sur Vercel (build local, on dépose `out/`)

Rien n'est construit chez Vercel : `next build` tourne sur ta machine (il lit
`../docs/legal/`, hors de ce projet) et seul le dossier `out/` est envoyé, comme
un site statique.

**Une seule fois**
```sh
npm i -g vercel && vercel login      # déjà fait si `vercel whoami` répond
cd marketing && vercel link          # crée/relie le projet, écrit .vercel/project.json
cp .env.production.example .env.production   # puis renseigner NEXT_PUBLIC_SITE_URL (https)
```

**À chaque mise en ligne**
```sh
npm run vercel:dry     # tout sauf l'envoi : vérifications, build, contrôle de out/
npm run vercel:preview         # aperçu (URL de preview) — à ouvrir et valider
npm run vercel:production    # production
```

Ce que `scripts/deploy.mjs` fait, dans l'ordre :
1. **Garde-fous** (production) : `NEXT_PUBLIC_SITE_URL` en https ; arbre git propre
   (`--allow-dirty` pour passer outre) ; textes légaux **sans** `[À COMPLÉTER]`
   (`--allow-draft-legal` pour publier quand même en « version provisoire »).
2. `typecheck` puis `next build` propres (`out/` et `.next/` supprimés avant).
3. **Contrôle de `out/`** : pages attendues présentes, aucune URL `localhost`,
   sitemap sur la bonne URL, alerte si des source maps sont publiques.
4. Copie `vercel.json` dans `out/` : il désactive tout build distant
   (`framework`, `buildCommand`, `installCommand` à `null`), ajoute les en-têtes de
   sécurité et le slash final (`/cgu` → `/cgu/`).
5. `vercel deploy out` (`--prod` en production), le lien projet passant par
   `VERCEL_ORG_ID` / `VERCEL_PROJECT_ID` lus dans `.vercel/project.json`.

Les `NEXT_PUBLIC_*` sont **figées dans le HTML au build** : changer l'URL de l'App
Store ou l'e-mail de contact demande un nouveau déploiement.

**Rollback** : `vercel rollback` (ou « Promote » d'un ancien déploiement dans le
tableau de bord) — chaque déploiement est immuable.

**Coût borné** : le site reste statique (pas de routes `app/api`, pas de
middleware, `images.unoptimized` volontaire). Ne pas y toucher sans mesurer.

**Domaine** : à choisir (`lagune.bailleul.dev` est déjà l'app) puis à ajouter dans
le projet Vercel (`vercel domains add`), avec `NEXT_PUBLIC_SITE_URL` identique.
