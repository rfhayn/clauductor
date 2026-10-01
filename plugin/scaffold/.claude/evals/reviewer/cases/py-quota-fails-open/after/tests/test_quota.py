import redis

from jobs import quota


class Down:
    def incr(self, key):
        raise redis.ConnectionError("down")


def test_a_redis_outage_does_not_crash_the_worker(monkeypatch):
    monkeypatch.setattr(quota, "r", Down())
    quota.within_quota("t1", 10)
