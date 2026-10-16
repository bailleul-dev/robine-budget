"""Contrat CLI pour l'appelant (aujourd'hui l'adaptateur Go).

Port-style : tout passe par stdin/stdout/l'environnement, rien par le
systeme de fichiers - comme un Port Erlang/Elixir, pas comme un programme
qu'on pointe vers un fichier d'etat partage.

Entrees :
  - identifiants : CCM_IDENTIFIER / CCM_PASSWORD dans l'environnement, voir
    config.load_credentials (inchange).
  - session (cookies / 2FA) : le contenu brut de stdin, chaine vide pour une
    toute premiere connexion (pas encore de session a reprendre).

Sortie : stdout ne contient QUE le JSON de resultat (une ligne), tout le
reste (messages de progression, "confirme sur ton appli mobile...") part sur
stderr - l'appelant lit stdout sans avoir a filtrer :
  - succes  : {"accounts": [...], "session": "..."}
  - echec   : {"error": "...", "detail": "...", "session": "..."} (session
    absent si rien n'a pu etre lu - voir sync.fail) - ecrit par sync.fail().

`--storage` a disparu du contrat avec l'appelant : sync.cmd_accounts en a
toujours besoin (l'API woob l'exige pour StandardStorage), mais ce n'est
plus qu'un fichier de travail scratch prive de cette CLI, cree et efface a
chaque appel - jamais quelque chose que l'appelant fournit, lit ou reutilise
d'un appel a l'autre.

Usage :
    echo -n "<session actuelle, ou rien>" | python -m bank_sync accounts

Toute erreur (attendue ou non) est capturee et redigee (redact()) avant
d'etre serialisee en JSON sur stdout par sync.fail() : rien ne doit jamais
pouvoir faire fuiter l'identifiant/mot de passe en clair via l'API, meme une
exception inattendue venant de woob/lxml/etc dont on ne maitrise pas le
message.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile

from .config import load_credentials
from .sync import cmd_accounts


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="bank-sync")
    subparsers = parser.add_subparsers(dest="command", required=True)
    subparsers.add_parser("accounts", help="Recupere les comptes et soldes")

    args = parser.parse_args(argv)

    if args.command != "accounts":
        parser.error(f"commande inconnue: {args.command}")
        return 1

    creds = load_credentials()
    session = sys.stdin.read()

    fd, storage_path = tempfile.mkstemp(prefix="bank-sync-")
    os.close(fd)
    try:
        result = cmd_accounts(storage_path, creds, session)
    finally:
        # cmd_accounts supprime deja le fichier sur le chemin succes -
        # best-effort ici pour les chemins d'erreur qui le laissent en
        # place (voir sync.fail, qui l'a deja lu avant qu'on arrive ici).
        try:
            os.remove(storage_path)
        except FileNotFoundError:
            pass

    if isinstance(result, int):
        # Erreur : sync.fail() a deja imprime le JSON redige sur stdout.
        return result

    print(json.dumps(result))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
