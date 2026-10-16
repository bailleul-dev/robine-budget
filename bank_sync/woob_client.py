"""Wrapper autour de l'API Python de woob (pas le CLI `woob`), pour pouvoir
etre appele en headless par un process parent (Go) sans terminal attache.

Sur la 2FA : Credit Mutuel impose une authentification forte (SCA). Ce module
ne "bypass" rien - il reproduit ce que fait le CLI `woob` en interactif (cf.
woob/tools/application/repl.py::_do_and_retry) pour que la validation par
appli mobile fonctionne sans planter avec NeedInteractiveFor2FA. Une reponse
par SMS/OTP (BrowserQuestion) n'est en revanche pas geree ici : en headless il
n'y a personne pour taper le code, donc on remonte une erreur explicite
(TwoFactorRequired) plutot que de bloquer sur un input().
"""

from __future__ import annotations

import sys
import time
from concurrent.futures import ThreadPoolExecutor
from concurrent.futures import TimeoutError as FutureTimeoutError
from contextlib import contextmanager
from dataclasses import dataclass
from decimal import Decimal
from typing import Iterator

from woob.core import Woob
from woob.exceptions import (
    AppValidationCancelled,
    AppValidationExpired,
    BrowserQuestion,
    DecoupledValidation,
    ModuleInstallError,
)
from woob.tools.backend import Module
from woob.tools.storage import StandardStorage

from .config import Credentials, redact

MODULE_NAME = "creditmutuel"
BACKEND_NAME = "cm"

# Borne haute sur l'attente d'une validation par appli mobile. Sans ca, un
# reseau qui ne repond jamais (ni AppValidationExpired ni rien d'autre) bloque
# le thread appelant indefiniment - inacceptable pour un endpoint REST qui
# doit repondre dans un delai borne.
DEFAULT_TWO_FACTOR_TIMEOUT = 120  # secondes


class TwoFactorRequired(RuntimeError):
    """Le module demande une reponse interactive (SMS/OTP) qu'on ne peut pas
    fournir en headless pour l'instant (pas de canal pour la renvoyer)."""


class BankAuthError(RuntimeError):
    """La validation (appli mobile) a ete refusee ou a expire."""


class TwoFactorTimeout(BankAuthError):
    """La validation (appli mobile) n'a pas ete confirmee dans le delai
    imparti (DEFAULT_TWO_FACTOR_TIMEOUT). Distinct de AppValidationExpired :
    ici c'est nous qui abandonnons, pas la banque qui nous dit que c'est
    expire - le cas qui couvre un reseau/serveur qui ne repond jamais."""


@dataclass(frozen=True)
class Transaction:
    id: str
    date: str  # ISO "YYYY-MM-DD"
    label: str
    amount: Decimal

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "date": self.date,
            "label": self.label,
            "amount": str(self.amount),
        }


@dataclass(frozen=True)
class Account:
    id: str
    label: str
    balance: Decimal
    currency: str | None
    operations: list[Transaction]

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "label": self.label,
            "balance": str(self.balance),
            "currency": self.currency or "EUR",
            "operations": [op.to_dict() for op in self.operations],
        }


def _enable_interactive_2fa(backend: Module) -> None:
    """Autorise le module a lever AppValidation/BrowserQuestion au lieu de
    NeedInteractiveFor2FA. Equivalent de ce que fait le CLI woob quand stdout
    est un tty - ici on l'active toujours, la validation appli n'a pas besoin
    d'un vrai terminal, juste d'attendre que le telephone confirme."""
    key = "request_information"
    if key in backend.config and backend.config[key].get() is None:
        backend.config[key].set({})


def _call_with_2fa(
    backend: Module,
    creds: Credentials,
    method_name: str,
    *args,
    timeout: float = DEFAULT_TWO_FACTOR_TIMEOUT,
    **kwargs,
) -> list:
    """creds is only used here to redact - login/password are never
    otherwise read in this function. Every message this loop can emit
    (exception text, stderr progress prints) is bank-provided or
    woob-provided, not ours to fully control, so everything gets redacted
    on principle rather than only the paths we currently think could
    contain a credential.

    Each attempt runs in a worker thread bounded by the time left on
    `timeout`: woob/the bank are trusted to eventually raise
    AppValidationExpired, but nothing guarantees that if the network just
    hangs, so we enforce our own hard deadline on top rather than relying
    on it. A future.result() timeout doesn't kill the worker thread (Python
    can't do that) - the pool is shut down with wait=False so we return to
    the caller immediately, leaving the thread to finish in the background
    against a backend/storage file the caller may already be closing. An
    accepted tradeoff for never blocking the caller past `timeout`; that's
    why the executor isn't a context manager here - its __exit__ would
    itself block until the thread finishes, defeating the point."""
    deadline = time.monotonic() + timeout
    method = getattr(backend, method_name)

    def call() -> list:
        # iter_accounts/iter_history are generators: calling method(...)
        # alone doesn't do any network I/O, it just builds the generator -
        # the 2FA prompt and the AppValidation/DecoupledValidation/etc raise
        # only happen once it's actually iterated. list(...) has to run
        # in here, inside the worker thread, or the timeout below only
        # bounds the (near-instant) generator creation and every exception
        # from the real work escapes uncaught, past every except clause.
        return list(method(*args, **kwargs))

    while True:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise TwoFactorTimeout(
                f"Delai d'attente ({timeout}s) depasse en attendant la validation 2FA.") from None
        pool = ThreadPoolExecutor(max_workers=1)
        future = pool.submit(call)
        try:
            result = future.result(timeout=remaining)
        except FutureTimeoutError:
            raise TwoFactorTimeout(
                f"Delai d'attente ({timeout}s) depasse en attendant la validation 2FA.") from None
        except BrowserQuestion as question:
            fields = ", ".join(
                field.label or field.id for field in question.fields)
            raise TwoFactorRequired(
                redact(fields, creds.login, creds.password)) from None
        except AppValidationCancelled:
            raise BankAuthError(
                "Validation refusee depuis l'application mobile.") from None
        except AppValidationExpired:
            raise BankAuthError(
                "Delai de validation depasse (pas confirme a temps dans l'appli).") from None
        except DecoupledValidation as validation:
            message = validation.message or "Confirme la connexion depuis ton application mobile..."
            print(redact(message, creds.login, creds.password), file=sys.stderr)
            if "resume" in backend.config:
                backend.config["resume"].set(True)
            continue
        finally:
            pool.shutdown(wait=False, cancel_futures=True)
        return result


def _load_backend(w: Woob, creds: Credentials, storage: StandardStorage) -> Module:
    params = {"login": creds.login, "password": creds.password}
    try:
        return w.load_backend(MODULE_NAME, BACKEND_NAME, params=params, storage=storage)
    except ModuleInstallError:
        # Module pas encore installe / keyring absent (~/.local/share/woob) :
        # on rafraichit les depots (index + trousseau GPG) une fois, comme le
        # fait `woob update`, puis on reessaie.
        print("Mise a jour des depots woob (module/keyring manquant)...",
              file=sys.stderr)
        w.repositories.update()
        return w.load_backend(MODULE_NAME, BACKEND_NAME, params=params, storage=storage)


@contextmanager
def backend_session(creds: Credentials, storage_path: str) -> Iterator[Module]:
    """Ouvre un backend sur le fichier d'etat woob donne (cookies + date de
    derniere validation 2FA). L'appelant est responsable de fournir ce chemin
    et d'en recuperer le contenu apres coup - ce module ne sait rien de
    Postgres ni de chiffrement, il lit/ecrit juste un fichier local."""
    storage = StandardStorage(storage_path)
    w = Woob()
    try:
        backend = _load_backend(w, creds, storage)
        _enable_interactive_2fa(backend)
        yield backend
    finally:
        w.unload_backends()  # declenche dump_state() -> ecrit storage_path


def _fetch_operations(backend: Module, creds: Credentials, raw_account) -> list[Transaction]:
    """Woob normalizes every bank to the same Transaction shape - unlike CSV
    import, there's no per-bank format/mapping to configure here, we just
    read woob's standard fields directly."""
    raw_transactions = _call_with_2fa(
        backend, creds, "iter_history", raw_account)
    operations: list[Transaction] = []
    for txn in raw_transactions:
        if txn.coming:
            # Not yet booked (e.g. a pending card hold) - iter_history
            # shouldn't return these, but skip defensively rather than
            # persist an amount/date that can still change.
            continue
        date = txn.date or txn.rdate
        if not date or txn.amount is None:
            continue
        operations.append(
            Transaction(
                id=txn.id or "",
                # [:10]: date can be a date or a datetime depending on the
                # module - always take just the "YYYY-MM-DD" part, the Go
                # side (pkg/civil) only parses that.
                date=date.isoformat()[:10],
                label=txn.label or txn.raw or "",
                amount=txn.amount,
            )
        )
    return operations


def fetch_accounts(creds: Credentials, storage_path: str) -> list[Account]:
    with backend_session(creds, storage_path) as backend:
        raw_accounts = _call_with_2fa(backend, creds, "iter_accounts")
        return [
            Account(
                id=raw_account.id,
                label=raw_account.label,
                balance=raw_account.balance,
                currency=getattr(raw_account, "currency", None),
                operations=_fetch_operations(backend, creds, raw_account),
            )
            for raw_account in raw_accounts
        ]
