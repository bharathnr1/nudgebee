"""Tests for the Discord recommendation / FinOps templates."""

from notifications_server.message_templates.slack.recommendation_proactive_nudge import ProactiveNudgeParams
from notifications_server.message_templates.slack.recommendation_resolution import (
    RecommendationResolutionParams,
    ResolutionDetails,
)
from notifications_server.message_templates.slack.recommendation_nudge_digest import (
    RecommendationNudgeDigestParams,
    AccountRecommendations,
    DigestRecommendation,
)
from notifications_server.message_templates.slack.cloud_cost_summary import (
    CloudCostSummary,
    CostSummary,
    CostPeriod,
    DailyItem,
    MonthlyService,
)
from notifications_server.message_templates.discord.recommendation_proactive_nudge import (
    get_discord_recommendation_proactive_nudge_template,
)
from notifications_server.message_templates.discord.recommendation_resolution import (
    get_discord_recommendation_resolution_template,
)
from notifications_server.message_templates.discord.recommendation_nudge_digest import (
    get_discord_recommendation_nudge_digest_template,
)
from notifications_server.message_templates.discord.cloud_cost_summary import (
    get_discord_cloud_cost_summary_template,
)


def test_proactive_nudge():
    p = ProactiveNudgeParams(
        organization_name="Acme", total_recommendations=7, total_recoverable_savings=1234.5, base_url="https://app.test"
    )
    payload = get_discord_recommendation_proactive_nudge_template(p)
    embed = payload["embeds"][0]
    assert "7" in embed["description"] and "Acme" in embed["description"]
    assert "$1,234.50" in embed["description"]
    assert embed["url"] == "https://app.test"


def test_resolution_with_details():
    p = RecommendationResolutionParams(
        resource_name="cart-api",
        rule_name="rightsize-cpu",
        category="RightSizing",
        account_name="prod",
        finops_score=82,
        finops_band="High",
        estimated_savings=99.0,
        severity="High",
        status="Resolved",
        resolution=ResolutionDetails(resolver="alice", type="manual", status_message="applied"),
    )
    payload = get_discord_recommendation_resolution_template(p)
    fields = {f["name"]: f["value"] for f in payload["embeds"][0]["fields"]}
    assert fields["Estimated Savings"] == "$99.00"
    assert fields["FinOps Score"] == "82 (High)"
    assert "alice" in fields["Resolution"]


def test_resolution_without_details_has_no_empty_fields():
    p = RecommendationResolutionParams(resource_name="x", rule_name="r", estimated_savings=0)
    payload = get_discord_recommendation_resolution_template(p)
    for f in payload["embeds"][0]["fields"]:
        assert f["value"] not in ("", None)


def test_cloud_cost_summary_currency_and_top_items():
    p = CloudCostSummary(
        account_id="acc-1",
        account_name="prod",
        period=CostPeriod(start="2026-06-01", end="2026-06-27"),
        title="Cloud Cost Summary",
        total_daily_cost=120.0,
        total_monthly_cost=3000.0,
        cost_currency="USD",
        summary=CostSummary(
            top_5_daily_items=[DailyItem(cost=50.0, product_code="EC2")],
            top_5_monthly_services=[MonthlyService(cost=2000.0, service="RDS")],
        ),
    )
    payload = get_discord_cloud_cost_summary_template(p)
    embed = payload["embeds"][0]
    assert "$120.00" in embed["description"] and "$3,000.00" in embed["description"]
    field_text = " ".join(f["value"] for f in embed["fields"])
    assert "EC2" in field_text and "RDS" in field_text


def test_nudge_digest_groups_accounts_and_caps_embeds():
    accounts = {
        f"acc-{i}": AccountRecommendations(
            account_name=f"acct-{i}",
            recommendations=[
                DigestRecommendation(
                    id=f"r{i}",
                    rule_name="rightsize",
                    resource_name="api",
                    finops_score=70,
                    finops_band="High",
                    estimated_savings=10.0,
                )
            ],
        )
        for i in range(15)
    }
    p = RecommendationNudgeDigestParams(
        organization_name="Acme",
        title="FinOps Daily Brief",
        total_recoverable_savings=500.0,
        act_now_count=3,
        recommendations_by_account=accounts,
    )
    payload = get_discord_recommendation_nudge_digest_template(p)
    assert payload["content"] == "FinOps Daily Brief"
    assert len(payload["embeds"]) <= 10  # summary + accounts, capped
    assert "$500.00" in payload["embeds"][0]["description"]


def test_money_helpers_are_none_safe():
    # The _money guards are defensive: render an em-dash for None rather than "$None"/"None".
    from notifications_server.message_templates.discord.cloud_cost_summary import _money as cost_money
    from notifications_server.message_templates.discord.recommendation_resolution import _money as res_money

    assert cost_money(None) == "—"
    assert res_money(None) == "—"


def test_empty_finops_band_renders_cleanly():
    # finops_band is a str field, so "" is reachable; it must not render "(...)"/"[]".
    r = get_discord_recommendation_resolution_template(
        RecommendationResolutionParams(resource_name="x", rule_name="r", finops_score=0, finops_band="")
    )
    rfields = {f["name"]: f["value"] for f in r["embeds"][0]["fields"]}
    assert rfields["FinOps Score"] == "0"  # no "(…)" appended when band is empty

    digest = get_discord_recommendation_nudge_digest_template(
        RecommendationNudgeDigestParams(
            organization_name="A",
            title="T",
            recommendations_by_account={
                "a": AccountRecommendations(
                    account_name="x",
                    recommendations=[
                        DigestRecommendation(
                            id="1", rule_name="rs", resource_name="api", finops_score=0, finops_band=""
                        )
                    ],
                )
            },
        )
    )
    body = digest["embeds"][1]["description"]
    assert "[]" not in body and "[None]" not in body
