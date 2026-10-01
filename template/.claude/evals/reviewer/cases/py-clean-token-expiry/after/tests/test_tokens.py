from auth.tokens import sign, verify

SECRET = b"k"


def test_sign_names_the_user():
    assert sign(SECRET, "u1", 2000).startswith("u1.2000.")


def test_valid_token_returns_its_user():
    assert verify(SECRET, sign(SECRET, "u1", 2000), now=1000) == "u1"


def test_tampered_token_is_rejected():
    token = sign(SECRET, "u1", 2000)
    assert verify(SECRET, "u2" + token[2:], now=1000) is None


def test_expired_token_is_rejected():
    token = sign(SECRET, "u1", 2000)
    assert verify(SECRET, token, now=2000) is None
    assert verify(SECRET, token, now=2001) is None


def test_malformed_token_is_rejected():
    assert verify(SECRET, "garbage", now=1000) is None
    assert verify(SECRET, "u1.soon.abc", now=1000) is None


def test_unicode_and_oversized_tokens_are_rejected_not_raised():
    token = sign(SECRET, "u1", 2000)
    assert verify(SECRET, "u1.²000." + token.rsplit(".", 1)[1], now=1000) is None
    assert verify(SECRET, "u1.2000.é" + token.rsplit(".", 1)[1][1:], now=1000) is None
    assert verify(SECRET, "\ud800.2000.abc", now=1000) is None
    assert verify(SECRET, "u1." + "9" * 5000 + ".abc", now=1000) is None
