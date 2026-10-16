# TASKS — ouverture publique de Lagune

Suivi des chantiers sécurité / conformité / hébergement avant d'ouvrir l'app à
tout le monde. Légende : `[x]` fait et testé, `[~]` fait en partie (voir la
note), `[ ]` à faire. Les sections « Reporté » et « Hors code » ne sont pas
traitées par le code.

## Bêta privée sur le NAS (décidé)

Le NAS suffit pour une bêta fermée de proches (moins de 20 personnes, prévenus que
c'est de l'auto-hébergé). Ce n'est **pas** acceptable pour le public (voir l'analyse
dans la conversation : disponibilité, sauvegarde hors site, exposition du LAN).

- [x] **Inscription fermée** : `SIGNUP_ALLOWED_EMAILS` (liste d'adresses, `*` = ouvert,
      vide = fermé ; défaut fermé en production). Bloque les 4 façons de créer un
      compte (mot de passe, Google, Apple, Robine) ; les comptes existants ne sont
      jamais touchés ; la réponse ne révèle pas quelles adresses ont un compte.
      Variable Ansible : `lagune_signup_allowed_emails`. Testé (service, config, HTTP).
- [ ] **Renseigner `lagune_signup_allowed_emails`** avec les adresses des invités
      (et la tienne si tu veux pouvoir recréer un compte). Un utilisateur Apple qui
      masque son adresse ne peut pas être reconnu : lui demander de se connecter avec
      son e-mail ou de partager son adresse.
- [x] Textes légaux : mention de la bêta privée et de l'auto-hébergement (véridiques
      aujourd'hui) ; à retirer à l'ouverture publique.
- [ ] **Sauvegarde chiffrée hors site** (ex. stockage objet Scaleway) + test de
      restauration : aujourd'hui tout est sur le disque du NAS.
- [ ] **Supervision et alertes** (disponibilité, disque, certificat) + savoir qui
      prévenir en cas de panne.
- [ ] Cloudflare devant l'app et tunnel sortant plutôt qu'un port ouvert sur la box
      (`lagune_trusted_proxy_hops: 2` avec Cloudflare).
- [ ] Onduleur pour le NAS ; idéalement isoler le serveur dans son VLAN.
- [ ] Prévenir les invités : bêta, auto-hébergée, données exportables, pas de garantie
      de disponibilité.

## À faire tout de suite au déploiement (sinon régression pour toi)

1. **`lagune_bank_sync_allowed_emails`** (`../silo01/roles/lagune/defaults/main.yml`)
   doit contenir **ton adresse e-mail** : la synchro bancaire est désormais
   fermée à tout le monde par défaut, toi compris. Le cron de 03:00 continue de
   synchroniser les connexions existantes ; c'est le bouton de l'app (et l'outil
   MCP `sync_bank`) qui passe par cette liste. Ton compte existant est
   automatiquement considéré « vérifié » par la migration 000021.
2. **`lagune_smtp_*`** : sans hôte SMTP, les e-mails de vérification et de
   réinitialisation **ne partent pas** (le serveur le signale au démarrage).
3. **Vérifier l'IP client vue par l'app** (rate limiting) : voir
   `../silo01/roles/lagune/README.md`, section « Security hardening ». Le Service
   Traefik est en `externalTrafficPolicy: Cluster`, qui peut masquer l'IP réelle ;
   si tous les refus `rate limit:` du log montrent la même adresse `10.42.x.x`,
   passer Traefik en `Local`.
4. **Reconstruire l'image** (`make docker-build` / push) : elle tourne maintenant en
   non-root, et l'init-container du manifest change le propriétaire de `/data`.
5. Rejouer `make lagune`.

## Reporté (décision de Guillaume)

- [ ] Rotation du secret OAuth Google + purge de l'historique git du fichier
      `client_secret_*.json` et de `frefreferferf.csv` (`git filter-repo`).
      À faire en tout dernier, avant toute publication du dépôt.
- [ ] CI de sécurité (`govulncheck`, `trivy`, `gitleaks`).
- Bank sync (woob) : reste réservé à un usage personnel (verrouillé côté
  serveur, voir §3). Pour le public : agrégateur agréé (Powens, Bridge,
  GoCardless) ou import CSV.

## 1. Configuration et secrets

- [x] Refus de démarrer en production avec `JWT_SECRET` / `BANK_SYNC_ENCRYPTION_KEY`
      par défaut ou trop courts, cookie non sécurisé, ou `SEED_DEV_USER`
      (`core.Settings.Validate`, appelée par `main`). Testé, y compris sur le binaire.
- [x] `APP_ENV` (défaut `production`) ; le dev met `APP_ENV=development`
      (déjà ajouté à `.env`, `backend/.env` et `.env.example`).
- [x] `COOKIE_SECURE` vaut `true` par défaut en production.
- [ ] Rotation des secrets après migration d'hébergeur : changer `JWT_SECRET`
      déconnecte tout le monde (voulu) ; changer `BANK_SYNC_ENCRYPTION_KEY`
      exige de re-chiffrer `bank_connections` d'abord.
- [ ] Secrets OIDC/Google : désormais dans le Secret k8s (plus en clair dans le
      Deployment). Le fichier `secrets/` d'Ansible reste à protéger/sauvegarder.

## 2. Authentification et sessions

- [x] Politique de mot de passe : 10 caractères minimum, 72 octets maximum
      (limite bcrypt), différent de l'e-mail. Vérifiée aussi côté mobile.
- [x] Login à temps constant (hash factice si l'e-mail est inconnu).
- [x] Rate limiting : 10 requêtes puis 1 toutes les 6 s **par IP** sur login,
      inscription, reset, vérification, OAuth (`/oauth/*`), Google/Apple mobile ;
      login : 5 échecs puis 1 toutes les 30 s **par (IP, e-mail)** (un succès
      ne consomme pas le quota, un attaquant ne verrouille pas le vrai
      propriétaire depuis une autre IP) ; envois d'e-mails plafonnés par adresse.
- [x] Sessions révocables : `users.token_version` + claim `ver` du JWT ;
      `POST /api/auth/logout-all` ; révocation après reset de mot de passe.
      Les jetons émis avant ce changement restent valides (version 0).
      S'applique aussi à l'endpoint MCP et aux pages OAuth.
- [x] Vérification d'e-mail : `email_verified_at`, e-mail au moment de l'inscription,
      page `/verify-email` (le GET n'affiche qu'un bouton, seul le POST consomme
      le jeton : les scanners de liens n'invalident pas le lien).
- [x] Reset de mot de passe : `POST /api/auth/forgot-password` (réponse identique
      que l'adresse existe ou non, traitement asynchrone), page `/reset-password`,
      lien valable 1 h, usage unique, jeton stocké haché (SHA-256).
- [x] **Anti pré-piratage de compte** : un compte mot de passe non vérifié n'est
      plus lié à un login Google/Apple/Robine sur l'e-mail (son mot de passe est
      effacé et ses sessions révoquées). Avant, quelqu'un pouvait s'inscrire avec
      l'adresse d'un tiers et hériter de ses données à sa première connexion Google.
- [x] **`email_verified` exigé des fournisseurs** : le callback Google web, Apple
      et Robine liaient les comptes sur un e-mail non contrôlé. Un e-mail que le
      fournisseur ne certifie pas est maintenant ignoré.
- [x] E-mails insensibles à la casse (connexion, reset, liaison) ; nouvelles
      adresses stockées en minuscules.
- [~] Énumération d'e-mails à l'inscription (« email already registered ») :
      atténuée par le rate limiting seulement. La supprimer impose un flux
      « vérifier d'abord » qui change l'UX mobile — décision produit à prendre.
- [ ] Changement d'e-mail / de mot de passe depuis l'app (aujourd'hui : reset par
      e-mail uniquement).
- [ ] Le callback Robine natif transmet le jeton de session en paramètre d'URL
      (`?token=`, vers le schéma d'app). Acceptable en deep-link, à revoir si ce
      flux passe un jour par un navigateur ordinaire (historique, logs Traefik).
- [ ] 2FA optionnelle (TOTP) — plus tard.

## 3. Surface HTTP et données

- [x] En-têtes : `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`,
      `Permissions-Policy`, HSTS (6 mois, si cookies sécurisés), `Cache-Control:
      no-store` et CSP `default-src 'none'` sur `/api/*` (sauf mises à jour OTA).
- [x] Corps de requête limités : 1 Mo (JSON), 40 Mo (uploads CSV) ; `ReadTimeout` /
      `IdleTimeout` du serveur.
- [x] IP client derrière proxy : `TRUSTED_PROXY_HOPS` (Traefik = 1), en-tête
      `X-Forwarded-For` lu depuis la droite, ignoré si non configuré.
- [x] **Bank sync verrouillé** : `BANK_SYNC_ALLOWED_EMAILS` (adresses *vérifiées*).
      Vide = fermé pour tous (routes HTTP, outil MCP `sync_bank`). `GET
      /api/bank-sync/status` renvoie `available` ; l'app masque la section.
- [x] Audit IDOR : voir la section dédiée, 2 failles corrigées.

## 4. Conformité (RGPD / App Store)

- [x] Export des données : `GET /api/auth/me/export` (JSON, pièce jointe, limité à
      3 par 10 min, sans hash de mot de passe, sans identifiants bancaires, sans
      jeton d'invitation, uniquement ce que l'utilisateur possède).
- [~] **Bouton d'export dans l'app** : l'API est prête, pas l'écran. Il faut
      `expo-file-system` + `expo-sharing` → nouvelle dépendance native, donc un
      nouveau build de l'app (pas seulement une mise à jour OTA).
- [x] Suppression de compte (existait : `DELETE /api/auth/me` ; cascade vérifiée,
      y compris les jetons d'authentification).
- [x] Politique de confidentialité et CGU **complétées** (`docs/legal/`) : éditeur
      Guillaume Bailleul (particulier), service gratuit, Scaleway (hébergement et
      e-mails), Vercel (site vitrine), 16 ans minimum, préavis 30 jours.
      Sans relecture d'un juriste. Elles **promettent des choses que
      l'infrastructure doit tenir** — voir le tableau de `docs/legal/README.md`.
- [ ] Tenir les promesses des textes avant l'ouverture : migration Scaleway faite,
      sauvegardes supprimées sous 3 mois (aujourd'hui Velero 15 j + restic 2
      mensuelles : OK, à revérifier après migration), **purge des logs à 30 jours**
      (rien ne l'impose aujourd'hui), SPF/DKIM/DMARC.
- [ ] Faire relire les documents légaux par un juriste (statut de particulier,
      non-publication de l'adresse, transfert vers Vercel).
- [ ] Mention / case d'acceptation des CGU à l'inscription dans l'app.
- [ ] Registre des traitements + liste des sous-traitants (hébergeur, SMTP).
- [ ] Procédure de notification de violation (72 h, CNIL).

## 5. Conteneur et Kubernetes (`../silo01/roles/lagune`)

Validé : rendu Jinja des templates, `kubectl apply --dry-run=client`,
`ansible-playbook --syntax-check`, `docker build --check` du Dockerfile, et la
couche non-root (uid 10001, écriture dans `/data`, `gpg`, `/tmp`) dans une
mini-image de test. **L'image complète n'a pas été reconstruite** (chaîne
npm / Go / uv) : à faire au premier `docker-build`, puis vérifier une synchro.

- [x] Image exécutée en non-root (`USER 10001`).
- [x] `securityContext` pod + conteneur : non-root, pas d'escalade, capabilities
      retirées, seccomp `RuntimeDefault`, pas de jeton de service-account.
- [x] Init-container qui donne `/data` à l'uid 10001 (les volumes hostPath
      ignorent `fsGroup`).
- [x] Requests/limits CPU + mémoire (768 Mi : le sous-processus woob est gourmand).
- [x] NetworkPolicies : l'app ne joint que DNS, son Postgres, Garage et Internet
      (443/587/465, jamais les CIDR du cluster) ; Postgres n'accepte que l'app.
- [x] `APP_ENV`, `COOKIE_SECURE`, `PUBLIC_URL`, `TRUSTED_PROXY_HOPS`, SMTP,
      `BANK_SYNC_ALLOWED_EMAILS` dans le manifest.
- [ ] `readOnlyRootFilesystem` : volontairement pas activé (le chemin woob n'a pas
      été éprouvé en lecture seule). À tenter avec un `emptyDir` sur `/tmp`.
- [ ] Aucune NetworkPolicy n'existait dans le cluster avant celle-ci : après
      déploiement, vérifier que les sondes, le login Google/Apple, l'OTA et une
      synchro fonctionnent (`k3s kubectl -n lagune get networkpolicy`).

## 6. Hors code (à faire par Guillaume)

- [ ] Choisir l'hébergeur (reco : Scaleway Paris, instance + PostgreSQL managé +
      Object Storage), Cloudflare devant (WAF / rate limiting de bord →
      `lagune_trusted_proxy_hops: 2`).
- [ ] Compte SMTP transactionnel (Scaleway TEM, Brevo, Postmark…) + SPF / DKIM /
      DMARC sur le domaine d'envoi.
- [ ] Tester une restauration Velero / dump Postgres de bout en bout.
- [ ] Supervision + alertes (uptime, 5xx, disque, certificat).

## 7. Site marketing (`marketing/`)

- [x] Landing complète (hero, fonctionnalités, étapes, prévisions, confiance, FAQ,
      CTA, pied de page), thème clair/sombre, responsive, export statique.
- [x] Pages `/confidentialite` et `/cgu` rendues depuis `docs/legal/` ; `noindex`
      et bandeau « version provisoire » tant qu'il reste des `[À COMPLÉTER]`.
- [x] Tableau comparatif par *type d'approche* (tableur / appli de banque / appli
      d'agrégation / Lagune), jamais par marque ; cartes empilées sur téléphone.
- [ ] **Quand la synchro bancaire est en service** : passer la ligne « Synchronisation
      automatique » de « En développement » à « Oui » dans
      `marketing/components/Compare.tsx`, et revoir la ligne « identifiants bancaires
      restent chez vous » (« Oui, aujourd'hui ») ainsi que la FAQ (`app/page.tsx`).
- [x] Flux de déploiement **build local + dépôt de `out/`** (`npm run vercel:preview`,
      `vercel:production`, `vercel:dry`) avec garde-fous ; testé jusqu'à l'envoi exclu.
- [ ] **Première mise en ligne** (jamais testée de bout en bout, l'envoi réel n'a
      pas été fait) : `cd marketing && vercel link`, renseigner `.env.production`,
      puis d'abord `npm run vercel:preview` (aperçu) pour vérifier que les pages, le
      slash final (`/cgu` → `/cgu/`), les en-têtes et la 404 sont corrects, avant
      `vercel:production`.
- [ ] Choisir le domaine de la vitrine (`lagune.bailleul.dev` est déjà l'app).
- [ ] Renseigner `NEXT_PUBLIC_APP_STORE_URL` à la publication (sinon « Bientôt sur iPhone »).
- [ ] Remplacer la maquette du hero par de vraies captures de l'app (`mobile/screenshots/`).
- [ ] Ajouter « hébergé en France / UE » **seulement** une fois l'hébergeur choisi.
- [ ] Image Open Graph dédiée pour le partage social.

## Audit IDOR

Méthode : lecture de chaque route `/api/...{id}` et de chaque identifiant reçu
dans un corps de requête, avec le mécanisme central (`accessibleAccountByID`,
`RequireOwnAccount`, `canAccessOwner`) : un accès hors périmètre répond 404
sans révéler l'existence de la ressource.

| Zone | Verdict |
|---|---|
| Comptes (get / delete), points de contrôle, import CSV, catégories, profils bancaires | OK (propriétaire vérifié ; import : profil global ou le sien) |
| Opérations (catégorie, découpage, commentaires), budgets récurrents, occurrences, prévisions | OK (via le compte de l'objet ; catégories limitées aux globales / à soi / au propriétaire du compte) |
| Partage (révocation) | OK (propriétaire de l'invitation) |
| Grants OAuth | OK (filtrés par utilisateur) |
| Outils MCP | OK (réutilisent les mêmes contrôles) |
| **`POST /api/reconciliation`** | **Faille corrigée** : `operation_id` (dans le corps) n'était pas contrôlé. On pouvait rapprocher sa propre occurrence avec l'opération d'un autre utilisateur : sa catégorie était **écrite** sur cette opération et son montant renvoyé. Désormais l'opération doit exister et appartenir au **même compte** que l'occurrence (404 sinon). Exploitation réaliste : un ex-collaborateur dont l'accès a été révoqué et qui connaît encore des identifiants d'opérations. Tests de non-régression ajoutés. |
| **`POST /api/invites/{token}/redeem`** | **Faille corrigée** : l'invitation est liée à une adresse e-mail, mais le compte comparé n'était pas vérifié — s'inscrire avec l'adresse de la personne invitée suffisait pour capter son accès partagé. Désormais l'e-mail doit être vérifié (403 sinon). Test de non-régression ajouté. |

Injections SQL : vérifié par recherche dans `adapters/db` — toutes les requêtes
sont paramétrées ; la seule construction dynamique est le numéro de placeholder
(`$N`) du `LIMIT` de la pagination des opérations, avec des arguments liés.

Non couvert : audit des dépendances (pas de CI, voir « Reporté »).
