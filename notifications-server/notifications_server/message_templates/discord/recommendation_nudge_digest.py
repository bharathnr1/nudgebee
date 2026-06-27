"""Discord renderer for the recommendation nudge digest (FinOps daily brief)."""

from typing import Any, Dict, List

from notifications_server.message_templates.slack.recommendation_nudge_digest import RecommendationNudgeDigestParams

DIGEST_COLOR = 3447003  # blue
MAX_RECS_PER_ACCOUNT = 5
MAX_EMBEDS = 10


def _money(amount: Any) -> str:
    if amount is None:
        return "—"
    try:
        return f"${float(amount):,.2f}"
    except (TypeError, ValueError):
        return str(amount)


def get_discord_recommendation_nudge_digest_template(params: RecommendationNudgeDigestParams) -> Dict[str, Any]:
    org = params.organization_name or "your organization"
    summary_embed: Dict[str, Any] = {
        "title": params.title,
        "color": DIGEST_COLOR,
        "description": (
            f"**{org}**\n"
            f"**Recoverable savings:** {_money(params.total_recoverable_savings)}\n"
            f"Act now: {params.act_now_count} • Critical: {params.critical_count} • High: {params.high_count}"
        ),
    }
    embeds: List[Dict[str, Any]] = [summary_embed]

    for acct in params.recommendations_by_account.values():
        if len(embeds) >= MAX_EMBEDS:
            break
        recs = acct.recommendations[:MAX_RECS_PER_ACCOUNT]
        if not recs:
            continue
        lines = []
        for r in recs:
            band = f" [{r.finops_band}]" if r.finops_band else ""
            lines.append(f"• **{r.rule_name}** (`{r.resource_name}`) — {_money(r.estimated_savings)}{band}")
        if len(acct.recommendations) > MAX_RECS_PER_ACCOUNT:
            lines.append(f"…and {len(acct.recommendations) - MAX_RECS_PER_ACCOUNT} more.")
        embeds.append(
            {"title": f"Account: {acct.account_name}", "description": "\n".join(lines)[:4000], "color": DIGEST_COLOR}
        )

    return {"content": params.title, "embeds": embeds}
