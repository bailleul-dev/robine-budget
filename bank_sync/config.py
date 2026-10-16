from __future__ import annotations

import os
import sys
from dataclasses import dataclass

from dotenv import load_dotenv


@dataclass(frozen=True)
class Credentials:
    login: str
    password: str


def redact(text: str, *secrets: str) -> str:
    """Replaces every occurrence of the given secrets in text with a
    placeholder. Defense in depth: applied to anything that leaves this
    process (stdout JSON, stderr progress messages) so a plaintext
    credential can never leak through an unexpected exception message, a
    library's own error text, or a bug - regardless of what actually threw
    it. Callers should apply this to every error/message they emit, not
    just the ones they expect to be able to contain a secret."""
    for secret in secrets:
        if secret:
            text = text.replace(secret, "***REDACTED***")
    return text


def load_credentials() -> Credentials:
    """Lit les identifiants depuis l'environnement.

    En local, `.env` les fournit via python-dotenv. En production, c'est
    l'appelant (le process Go) qui les injecte directement dans l'environnement
    du sous-processus - load_dotenv() ne les ecrase pas s'ils sont deja
    presents, donc le meme code marche dans les deux cas.
    """
    load_dotenv()
    login = os.environ.get("CCM_IDENTIFIER")
    password = os.environ.get("CCM_PASSWORD")
    if not login or not password:
        sys.exit("CCM_IDENTIFIER / CCM_PASSWORD manquants (.env ou variables d'environnement)")
    return Credentials(login=login, password=password)
