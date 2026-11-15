import json
from decimal import Decimal, InvalidOperation
from pathlib import Path

from .usage import Price


def load_table() -> dict:
    return json.loads(Path(__file__).with_name("prices.json").read_text(encoding="utf-8"))


def provider_for(model: str) -> str:
    return "anthropic" if model.startswith("claude-") else "openai"


def load_prices(models: list[str]) -> dict[str, Price]:
    table = load_table()
    prices = {}
    for model in models:
        row = table.get(model)
        if row is None:
            raise ValueError("no price row for the selected model; add one before a paid run")
        try:
            values = [Decimal(row[k]) for k in ("input", "output", "cache_read", "cache_write")]
        except TypeError, InvalidOperation:
            raise ValueError("model price row is missing or invalid") from None
        if (
            row.get("provider") != provider_for(model)
            or not row.get("source_url")
            or any(not v.is_finite() or v < 0 for v in values)
        ):
            raise ValueError("model price row is missing or invalid")
        prices[model] = Price(*values)
    return prices
