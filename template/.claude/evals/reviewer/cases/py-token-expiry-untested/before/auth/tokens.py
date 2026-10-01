import hashlib
import hmac


def sign(secret: bytes, user_id: str, expires_at: int) -> str:
    """A token for user_id valid until expires_at (unix seconds)."""
    msg = f"{user_id}.{expires_at}".encode()
    return f"{user_id}.{expires_at}." + hmac.new(secret, msg, hashlib.sha256).hexdigest()
