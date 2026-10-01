from api.pages import page


def test_second_page():
    assert page(list(range(5)), 2, 2) == [2, 3]


def test_last_partial_page():
    assert page(list(range(5)), 3, 2) == [4]
