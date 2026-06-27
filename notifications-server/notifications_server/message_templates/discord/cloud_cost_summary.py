"""Discord renderer for the cloud cost summary."""

from typing import Any, Dict, List

from notifications_server.message_templates.slack.cloud_cost_summary import CloudCostSummary, CURRENCY_SYMBOLS

COST_COLOR = 15844367  # gold


def _money(amount: Any, currency: str = "USD") -> str:
    symbol = CURRENCY_SYMBOLS.get(currency, "")
    try:
        return f"{symbol}{float(amount):,.2f}"
    except (TypeError, ValueError):
        return f"{symbol}{amount}"


def _items_field(title: str, lines: List[str]) -> Dict[str, Any]:
    return {"name": title, "value": "\n".join(lines)[:1024] or "—", "inline": False}


def get_discord_cloud_cost_summary_template(params: CloudCostSummary) -> Dict[str, Any]:
    currency = params.cost_currency
    account = params.account_name or params.account_id
    description = (
        f"**Account:** {account}\n"
        f"**Period:** {params.period.start} → {params.period.end}\n"
        f"**Daily:** {_money(params.total_daily_cost, currency)}  •  "
        f"**Monthly:** {_money(params.total_monthly_cost, currency)}"
    )
    fields: List[Dict[str, Any]] = []
    daily = params.summary.top_5_daily_items
    if daily:
        fields.append(
            _items_field("Top daily costs", [f"• {_money(i.cost, currency)} — {i.product_code}" for i in daily[:5]])
        )
    monthly = params.summary.top_5_monthly_services
    if monthly:
        fields.append(
            _items_field("Top monthly services", [f"• {_money(s.cost, currency)} — {s.service}" for s in monthly[:5]])
        )

    title = params.title or "Cloud Cost Summary"
    embed = {"title": title, "description": description, "color": COST_COLOR, "fields": fields}
    return {"content": title, "embeds": [embed]}
