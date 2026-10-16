# Politique de confidentialité — Lagune

*Dernière mise à jour : 23 septembre 2026*

## 1. Qui est responsable de vos données ?

Le responsable du traitement est **Guillaume Bailleul**, personne physique, éditeur de Lagune.
Contact pour toute question relative à vos données : guillaume.bailleul@gmail.com.

## 2. Quelles données collectons-nous ?

**Données de compte** : adresse e-mail (facultative si vous vous connectez avec Apple et masquez votre adresse), nom affiché, mot de passe (jamais stocké en clair : uniquement une empreinte irréversible), et, si vous utilisez la connexion Google, Apple ou Robine ID, l'identifiant que ce fournisseur nous transmet.

**Données financières que vous saisissez ou importez** : comptes, soldes, opérations (date, libellé, montant), catégories, budgets et opérations récurrentes, commentaires, fichiers d'export bancaire que vous importez (CSV).

**Données de partage** : adresse e-mail des personnes que vous invitez à consulter vos comptes, et état de l'invitation.

**Données techniques** : adresse IP et journaux du serveur (nécessaires à la sécurité et à la limitation des abus), cookie de session.

Nous ne collectons **aucune donnée à des fins publicitaires** et n'utilisons aucun traceur publicitaire ou de mesure d'audience tiers dans l'application.

## 3. Pourquoi, et sur quelle base légale ?

| Finalité | Base légale |
|---|---|
| Créer et gérer votre compte, fournir le service (suivi de budget, catégorisation, prévisions, partage) | Exécution du contrat (art. 6.1.b RGPD) |
| Vous envoyer les e-mails indispensables (confirmation d'adresse, réinitialisation de mot de passe) | Exécution du contrat |
| Sécuriser le service (limitation des tentatives de connexion, détection d'abus, journaux) | Intérêt légitime (art. 6.1.f) |
| Répondre à vos demandes d'exercice de droits, respecter nos obligations légales | Obligation légale (art. 6.1.c) |

## 4. Avec qui partageons-nous vos données ?

Uniquement avec les prestataires strictement nécessaires (sous-traitants) :

- **Hébergement de l'application et de sa base de données** : Scaleway SAS (France) — serveurs situés dans l'Union européenne. **Pendant la bêta privée**, l'application est en revanche auto-hébergée par l'éditeur, en France, sur sa propre infrastructure ; elle sera migrée chez Scaleway à l'ouverture au public, et cette politique sera alors mise à jour.
- **Envoi d'e-mails** (confirmation d'adresse, réinitialisation de mot de passe) : Scaleway (service Transactional Email, Union européenne) — uniquement votre adresse e-mail et le contenu du message transactionnel.
- **Hébergement du site vitrine** (voir § 9) : Vercel Inc. (États-Unis).
- **Connexion tierce (facultative)** : Google, Apple — uniquement si vous choisissez ce mode de connexion ; ils nous transmettent un identifiant et, selon vos réglages, votre e-mail et votre nom.

Les personnes que **vous** invitez à partager un compte voient les données de ce compte (opérations, catégories, commentaires) tant que vous ne révoquez pas l'accès.

Vos données ne sont ni vendues ni cédées à des tiers.

## 5. Où sont-elles stockées, et combien de temps ?

Les données de l'application sont stockées en France (chez l'éditeur pendant la bêta privée, puis chez Scaleway, région de Paris, à l'ouverture au public), donc dans l'Union européenne.

- Vos données sont conservées **tant que votre compte existe**.
- **Suppression du compte** depuis l'application (Réglages → Supprimer mon compte) : l'ensemble de vos données est effacé immédiatement de la base de production.
- Les **sauvegardes** techniques contenant ces données sont stockées dans l'Union européenne et supprimées automatiquement : une donnée effacée de la production disparaît donc aussi des sauvegardes **au plus tard sous trois mois**.
- Les **journaux techniques** du serveur (adresse IP, requêtes) sont conservés **30 jours au plus**.

## 6. Vos droits

Vous pouvez à tout moment exercer vos droits d'accès, de rectification, d'effacement, de limitation, d'opposition et de portabilité.

- **Accès et portabilité** : l'application permet d'obtenir un export complet de vos données au format JSON (route `GET /api/auth/me/export`).
- **Effacement** : suppression du compte depuis l'application.
- **Autres demandes** : écrivez à guillaume.bailleul@gmail.com. Nous répondons sous un mois.
- **Réclamation** : vous pouvez saisir la CNIL (www.cnil.fr) si vous estimez que vos droits ne sont pas respectés.

## 7. Sécurité

Nous mettons en œuvre des mesures adaptées : connexion chiffrée (HTTPS), mots de passe protégés par un hachage robuste (bcrypt), sessions révocables (« se déconnecter de tous les appareils »), limitation des tentatives de connexion, cloisonnement des données entre utilisateurs, conteneurs exécutés sans privilèges, sauvegardes.
Aucun système n'étant infaillible, nous vous informons — ainsi que la CNIL lorsque la loi l'exige — de toute violation de données susceptible de vous exposer à un risque.

## 8. Synchronisation bancaire

Cette fonctionnalité n'est **pas proposée au public**. Si elle l'est un jour, elle passera par un prestataire d'agrégation agréé et cette politique sera mise à jour avant toute ouverture.

## 9. Cookies et site vitrine

L'application n'utilise qu'un **cookie de session strictement nécessaire** au fonctionnement (authentification), exempté de consentement. Aucun cookie publicitaire ni de mesure d'audience.

Le **site vitrine** (pages de présentation, politique de confidentialité, conditions d'utilisation) ne dépose aucun cookie et n'utilise aucun outil de mesure d'audience. Il est hébergé par Vercel Inc., prestataire établi aux États-Unis, qui traite techniquement l'adresse IP des visiteurs pour délivrer les pages et assurer leur sécurité. Ce transfert est encadré par les garanties prévues par le RGPD (cadre de protection des données UE–États-Unis et/ou clauses contractuelles types de la Commission européenne). Le site vitrine ne contient aucune donnée de votre compte.

## 10. Modifications

Nous vous informerons dans l'application de toute modification importante de cette politique ; la date de dernière mise à jour figure en tête de ce document.
