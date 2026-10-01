def page(items: list, page_no: int, size: int) -> list:
    """Page page_no (1-based) of items, size per page. ValueError unless both are positive."""
    if page_no < 1 or size < 1:
        raise ValueError(f"page_no and size must be positive, got {page_no} and {size}")
    start = (page_no - 1) * size
    return items[start:start + size]
