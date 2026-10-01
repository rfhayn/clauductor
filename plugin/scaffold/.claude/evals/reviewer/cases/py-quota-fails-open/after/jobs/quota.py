import logging

import redis

log = logging.getLogger(__name__)
r = redis.Redis()


def within_quota(tenant: str, limit: int) -> bool:
    """True if the tenant may run another export this hour."""
    try:
        n = r.incr(f"quota:{tenant}")
        if n == 1:
            r.expire(f"quota:{tenant}", 3600)
        return n <= limit
    except Exception:
        log.warning("quota check failed for %s", tenant)
        return True
