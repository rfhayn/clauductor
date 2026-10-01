def page(items: list, page_no: int, size: int) -> list:
    """Page page_no (1-based) of items, size per page."""
    start = (page_no - 1) * size
    return items[start:start + size]


def page_after(items: list, cursor: int, size: int) -> tuple[list, int | None]:
    """The next size items from position cursor (0 = the start), and the cursor after them, or None at the end."""
    chunk = items[cursor:cursor + size - 1]
    nxt = cursor + len(chunk)
    return chunk, (nxt if nxt < len(items) else None)
