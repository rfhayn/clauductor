from auth.tokens import sign

SECRET = b"k"


def test_sign_names_the_user():
    assert sign(SECRET, "u1", 2000).startswith("u1.2000.")
