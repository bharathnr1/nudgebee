"""Discord renderer for recommendation-resolution notifications."""

from typing import Any, Dict

from notifications_server.message_templates.slack.recommendation_resolution import RecommendationResolutionParams

RESOLUTION_COLOR = 3066993  # green


def _money(amount: Any) -> str:
    if amount is None:
        return "—"
    try:
        return f"${float(amount):,.2f}"
    except (TypeError, ValueError):
        return str(amount)


def _v(value: Any) -> str:
    return str(value) if value not in (None, "") else "—"


def get_discord_recommendation_resolution_template(params: RecommendationResolutionParams) -> Dict[str, Any]:
    description = (
        f"**Resource:** `{_v(params.resource_name)}`\n"
        f"**Rule:** {_v(params.rule_name)}\n"
        f"**Account:** {_v(params.account_name)}"
    )
    finops_score_val = "—"
    if params.finops_score is not None and params.finops_band:
        finops_score_val = f"{params.finops_score} ({params.finops_band})"
    elif params.finops_score is not None:
        finops_score_val = str(params.finops_score)
    elif params.finops_band:
        finops_score_val = str(params.finops_band)

    fields = [
        {"name": "Status", "value": _v(params.status), "inline": True},
        {"name": "Category", "value": _v(params.category), "inline": True},
        {"name": "Severity", "value": _v(params.severity), "inline": True},
        {"name": "FinOps Score", "value": finops_score_val, "inline": True},
        {"name": "Estimated Savings", "value": _money(params.estimated_savings), "inline": True},
    ]
    resolution = params.resolution
    if resolution is not None:
        detail = ", ".join(x for x in [resolution.resolver, resolution.type, resolution.status_message] if x)
        if detail:
            fields.append({"name": "Resolution", "value": detail[:1024], "inline": False})

    embed = {
        "title": "✅ Recommendation resolved",
        "description": description,
        "color": RESOLUTION_COLOR,
        "fields": fields,
    }
    return {"content": "✅ Recommendation resolved", "embeds": [embed]}
