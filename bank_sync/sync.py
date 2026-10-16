from .config import redact, Credentials
from .woob_client import BankAuthError, TwoFactorRequired, TwoFactorTimeout, fetch_accounts
import json
import os
import sys


def describe_session(raw: str) -> str:
    """Resume non sensible d'un etat woob (aucune valeur de cookie/token) :
    sert a comprendre pourquoi la 2FA est redemandee - l'etat entier est
    ignore par woob une fois `expire` depasse, et `expire` ne vaut plus que
    5 min sans cookie auth_client_state date."""
    if not raw:
        return "vide"
    try:
        data = json.loads(raw)
    except ValueError:
        return f"illisible ({len(raw)} octets)"
    found: dict = {}

    def walk(node):
        if isinstance(node, dict):
            for key, value in node.items():
                if key == "expire":
                    found["expire"] = value
                elif key == "twofa_auth_state":
                    found["twofa_auth_state"] = (
                        {"has_value": bool(value.get("value")), "expires": value.get("expires")}
                        if isinstance(value, dict) and value else "absent/vide")
                elif key == "twofa_logged_date":
                    found[key] = value
                else:
                    walk(value)
        elif isinstance(node, list):
            for item in node:
                walk(item)

    walk(data)
    return json.dumps(found)


def cmd_accounts(storage_path: str, creds: Credentials, session: str) -> int:
    print(f"bank_sync: session recue: {describe_session(session)}", file=sys.stderr)
    try:
        with open(storage_path, "w") as f:
            f.write(session)
        accounts = fetch_accounts(creds, storage_path)

        with open(storage_path) as f:
            s = f.read()
        print(f"bank_sync: session apres sync: {describe_session(s)}", file=sys.stderr)
        os.remove(storage_path)
    except TwoFactorRequired as exc:
        return fail("two_factor_required", str(exc), creds, storage_path, 2)
    except TwoFactorTimeout as exc:
        return fail("two_factor_timeout", str(exc), creds, storage_path, 3)
    except BankAuthError as exc:
        return fail("bank_auth_error", str(exc), creds, storage_path, 1)
    except Exception as exc:  # noqa: BLE001 - deliberately broad, see module docstring
        # Anything else (a woob/lxml/network error, a bug here) must still
        # come out as redacted JSON on stdout, never as Python's own
        # unhandled-exception traceback on stderr, which we don't control
        # the wording of and can't guarantee is credential-free.
        return fail("bank_error", f"{type(exc).__name__}: {exc}", creds, storage_path, 1)

    return {"accounts": [account.to_dict()
                         for account in accounts], "session": s}


def fail(error: str, detail: str, creds, storage_path: str, code: int) -> int:
    # woob_client.backend_session flushes whatever cookie/2FA state exists
    # to storage_path in a `finally`, even when the call above raised - so
    # even a failed attempt (an app-validation refused, a 2FA wait that
    # timed out) can leave real progress on disk worth handing back to the
    # caller, same as a success does. Best-effort: a storage_path that was
    # never created (failure before the first write) just means no session
    # to report, not an error in its own right.
    payload = {"error": error, "detail": redact(
        detail, creds.login, creds.password)}
    try:
        with open(storage_path) as f:
            payload["session"] = f.read()
    except OSError:
        pass
    # Second (belt-and-suspenders) redaction pass: woob_client.py already
    # redacts what it controls, but this is the last line before anything
    # reaches stdout - so it runs on every error path, including one this
    # module didn't anticipate.
    print(json.dumps(payload))
    return code
