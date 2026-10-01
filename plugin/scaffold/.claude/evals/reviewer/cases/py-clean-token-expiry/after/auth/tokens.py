import hashlib
import hmac


def sign(secret: bytes, user_id: str, expires_at: int) -> str:
    """A token for user_id valid until expires_at (unix seconds)."""
    msg = f"{user_id}.{expires_at}".encode()
    return f"{user_id}.{expires_at}." + hmac.new(secret, msg, hashlib.sha256).hexdigest()


def verify(secret: bytes, token: str, now: int) -> str | None:
    """The user id of a well-signed token that has not expired at now, else None."""
    try:
        user_id, expires_at, mac = token.rsplit(".", 2)
    except ValueError:
        return None
    # isdigit() alone admits Unicode digits ("²") that int() rejects, and compare_digest raises on
    # a non-ASCII str: an attacker's token must be rejected, never crash the caller.
    if not (expires_at.isascii() and expires_at.isdigit() and len(expires_at) <= 19 and mac.isascii()):
        return None
    try:
        good = sign(secret, user_id, int(expires_at)).rsplit(".", 1)[1]
    except UnicodeEncodeError:  # a user id sign() could never have issued (a lone surrogate)
        return None
    if not hmac.compare_digest(mac, good):
        return None
    if int(expires_at) <= now:
        return None
    return user_id
