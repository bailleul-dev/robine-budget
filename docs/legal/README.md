# Documents légaux

`privacy-policy.fr.md` (politique de confidentialité) et `terms.fr.md` (CGU) sont
**complets** et décrivent ce que l'application fait réellement (voir `TASKS.md`).
Ils ont été rédigés à partir du code et de l'infrastructure, **sans relecture d'un
juriste** : c'est recommandé avant d'ouvrir au grand public.

Le site (`marketing/`) les rend au build ; tant qu'un `[À COMPLÉTER]` y réapparaît,
la page passe en « version provisoire » (`noindex`, hors sitemap) et
`npm run vercel:production` refuse de publier.

## Ce qui doit rester vrai le jour de la mise en ligne

Les textes promettent des choses que l'infrastructure doit tenir. Les vérifier :

| Promesse dans les textes | À faire / à vérifier |
|---|---|
| **Bêta privée** : application auto-hébergée par l'éditeur ; **Scaleway (Paris)** à l'ouverture | Inscription fermée (`lagune_signup_allowed_emails`). À l'ouverture publique : migration terminée, puis **retirer les mentions « bêta privée »** des deux textes |
| E-mails envoyés par **Scaleway Transactional Email** | Domaine d'envoi configuré (SPF / DKIM / DMARC), `lagune_smtp_*` renseignés |
| Sauvegardes supprimées **sous trois mois** | Rétention réelle ≤ 3 mois après migration (aujourd'hui : Velero 15 j, restic 15 quotidiennes + 2 mensuelles) |
| Journaux serveur conservés **30 jours au plus** | Rotation / purge des logs à mettre en place (rien ne l'impose aujourd'hui) |
| Site vitrine chez **Vercel** (États-Unis), sans cookie ni mesure d'audience | Ne pas ajouter d'outil d'analyse sans mettre à jour la politique |
| Accès, export et suppression depuis l'app | Bouton d'export encore absent de l'app (API prête) |
| Âge minimum **16 ans**, préavis de fin de service **30 jours** | Valeurs choisies par défaut : à confirmer |
| Service **gratuit** | Si une offre payante arrive : prix, rétractation et résiliation à ajouter aux CGU |

## À faire évoluer avec le produit

- Synchronisation bancaire ouverte au public : la politique (§ 8) et les CGU doivent
  être mises à jour **avant**, avec le prestataire d'agrégation comme sous-traitant.
- Nouveau sous-traitant, nouvelle donnée collectée, outil de mesure d'audience :
  mettre à jour les documents dans le même changement.
- Si l'activité devient professionnelle (micro-entreprise, société) : identité de
  l'éditeur, mentions légales complètes (SIREN, adresse) et médiateur de la
  consommation à ajouter.

## Points à faire confirmer par un juriste

- Un particulier peut ne pas publier son adresse s'il la communique à l'hébergeur
  (LCEN, art. 6) : vérifier que c'est bien fait auprès de Scaleway et de Vercel.
- Les coordonnées complètes des hébergeurs (adresse, téléphone) : les CGU renvoient
  à leurs sites ; les recopier telles quelles depuis leurs mentions légales.
- Le cadre du transfert vers Vercel (États-Unis) : confirmer la certification au
  cadre de protection des données UE–États-Unis, ou les clauses contractuelles types.
