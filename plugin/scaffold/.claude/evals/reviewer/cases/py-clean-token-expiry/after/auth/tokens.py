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
    if not expires_at.isdigit():
        return None
    good = sign(secret, user_id, int(expires_at)).rsplit(".", 1)[1]
    if not hmac.compare_digest(mac, good):
        return None
    if int(expires_at) <= now:
        return None
    return user_id
