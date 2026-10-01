import pytest

from api.pages import page


def test_second_page():
    assert page(list(range(5)), 2, 2) == [2, 3]


def test_last_partial_page():
    assert page(list(range(5)), 3, 2) == [4]


@pytest.mark.parametrize("page_no,size", [(0, 2), (-1, 2), (1, 0), (1, -3)])
def test_rejects_a_non_positive_page_or_size(page_no, size):
    with pytest.raises(ValueError):
        page([1, 2, 3], page_no, size)
