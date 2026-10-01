def page(items: list, page_no: int, size: int) -> list:
    """Page page_no (1-based) of items, size per page."""
    start = (page_no - 1) * size
    return items[start:start + size]
