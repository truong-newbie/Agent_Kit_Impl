import re

def slugify(text: str) -> str:
    # Intentional bug: leaves leading/trailing hyphens and repeated hyphens.
    cleaned = re.sub(r"[^a-zA-Z0-9]+", "-", text.lower())
    return cleaned
