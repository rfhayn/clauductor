from api.pages import page, page_after


def test_second_page():
    assert page(list(range(5)), 2, 2) == [2, 3]


def test_last_partial_page():
    assert page(list(range(5)), 3, 2) == [4]


def test_page_after_walks_everything():
    items = list(range(5))
    seen, cursor = [], 0
    while cursor is not None:
        chunk, cursor = page_after(items, cursor, 2)
        seen += chunk
    assert seen == items
