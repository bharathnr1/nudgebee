"""Discord renderer for the proactive recommendation nudge."""

from typing import Any, Dict

from notifications_server.message_templates.slack.recommendation_proactive_nudge import ProactiveNudgeParams

REC_COLOR = 3447003  # blue


def _money(amount: Any) -> str:
    if amount is None:
        return "—"
    try:
        return f"${float(amount):,.2f}"
    except (TypeError, ValueError):
        return str(amount)


def get_discord_recommendation_proactive_nudge_template(params: ProactiveNudgeParams) -> Dict[str, Any]:
    org = params.organization_name or "your organization"
    description = (
        f"**{params.total_recommendations}** open optimization recommendations for **{org}**\n"
        f"**Recoverable savings:** {_money(params.total_recoverable_savings)}"
    )
    embed: Dict[str, Any] = {
        "title": "💡 New optimization recommendations",
        "description": description,
        "color": REC_COLOR,
    }
    if params.base_url:
        embed["url"] = params.base_url
    return {"content": "💡 New optimization recommendations", "embeds": [embed]}
