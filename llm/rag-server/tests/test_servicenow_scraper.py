"""Tests for ServiceNow KB extension-table bulk fetching in ``scraper.py``.

Regression coverage for #301: ``collect_servicenow_kb_documents`` used to issue
one extension-table GET per empty-bodied article (an O(N) N+1). It now batches
those into one ``sys_idIN`` query per extension class. These tests use a fake
pysnow client and assert call counts + resolved bodies — no live ServiceNow.
"""

from pysnow import QueryBuilder

from rag.core.documents import scraper

KNOWN_ERROR = "kb_template_known_error_article"
HOW_TO = "kb_template_how_to_article"


class FakeResponse:
    def __init__(self, records):
        self._records = records

    def all(self):
        return list(self._records)

    def one_or_none(self):
        return self._records[0] if self._records else None


class FakeResource:
    """Routes ``get`` by table and classifies the query as bulk vs single."""

    def __init__(self, table, store, call_log, fail_bulk_classes):
        self.table = table
        self.store = store
        self.call_log = call_log
        self.fail_bulk_classes = fail_bulk_classes

    def get(self, query=None, limit=None, offset=None):
        if self.table == "kb_knowledge":
            # Paginated published-article fetch: everything on page 1.
            return FakeResponse([] if offset else self.store.get("kb_knowledge", []))

        if isinstance(query, QueryBuilder):
            self.call_log.append((self.table, "bulk"))
            if self.table in self.fail_bulk_classes:
                raise RuntimeError("simulated bulk failure")
            wanted = set(str(query).split("IN", 1)[1].split(","))
            rows = [r for r in self.store.get(self.table, []) if r.get("sys_id") in wanted]
            return FakeResponse(rows)

        # dict query -> single-article fetch
        self.call_log.append((self.table, "single"))
        sid = (query or {}).get("sys_id")
        rows = [r for r in self.store.get(self.table, []) if r.get("sys_id") == sid]
        return FakeResponse(rows)


class FakeClient:
    def __init__(self, store, fail_bulk_classes=()):
        self.store = store
        self.call_log = []
        self.fail_bulk_classes = set(fail_bulk_classes)

    def resource(self, api_path):
        table = api_path.rsplit("/", 1)[-1]
        return FakeResource(table, self.store, self.call_log, self.fail_bulk_classes)


def _article(sys_id, sys_class_name, text="", wiki=""):
    return {
        "sys_id": sys_id,
        "sys_class_name": sys_class_name,
        "text": text,
        "wiki": wiki,
        "number": sys_id,
        "short_description": f"desc-{sys_id}",
        "keywords": "",
        "article_type": "",
        "sys_updated_on": "",
    }


def _ext_row(sys_id, **kb_fields):
    row = {"sys_id": sys_id}
    row.update(kb_fields)
    return row


def _bodies_by_sys_id(documents):
    return {d.metadata["sys_id"]: d.page_content for d in documents}


def _ext_calls(call_log):
    return [c for c in call_log if c[0] in (KNOWN_ERROR, HOW_TO)]


def test_bulk_fetch_one_query_per_class_not_per_article():
    """Empty-bodied articles are batched into one bulk query per class."""
    store = {
        "kb_knowledge": [
            _article("k1", KNOWN_ERROR),
            _article("k2", KNOWN_ERROR),
            _article("k3", KNOWN_ERROR),
            _article("h1", HOW_TO),
            _article("h2", HOW_TO),
            _article("n1", "kb_knowledge", text="<p>Normal body content</p>"),
        ],
        KNOWN_ERROR: [
            _ext_row("k1", kb_cause="Cause one"),
            _ext_row("k2", kb_workaround="Workaround two"),
            _ext_row("k3", kb_description="Description three"),
        ],
        HOW_TO: [
            _ext_row("h1", kb_question="Question one", kb_answer="Answer one"),
            _ext_row("h2", kb_answer="Answer two"),
        ],
    }
    client = FakeClient(store)

    documents = scraper.collect_servicenow_kb_documents(client, "https://dev.service-now.com")

    # Exactly one bulk query per class — not one per article (would be 5).
    assert _ext_calls(client.call_log) == [(KNOWN_ERROR, "bulk"), (HOW_TO, "bulk")]
    assert len(documents) == 6

    bodies = _bodies_by_sys_id(documents)
    assert "Cause one" in bodies["k1"]
    assert "Workaround two" in bodies["k2"]
    assert "Description three" in bodies["k3"]
    assert "Question one" in bodies["h1"] and "Answer one" in bodies["h1"]
    assert "Answer two" in bodies["h2"]
    assert "Normal body content" in bodies["n1"]


def test_normal_articles_skip_extension_fetch_entirely():
    """Articles with a non-empty primary body never touch the extension table."""
    store = {"kb_knowledge": [_article("n1", "kb_knowledge", text="<p>Body</p>")]}
    client = FakeClient(store)

    documents = scraper.collect_servicenow_kb_documents(client, "https://dev.service-now.com")

    assert len(documents) == 1
    assert _ext_calls(client.call_log) == []


def test_empty_extension_article_makes_no_redundant_single_fetch():
    """A genuinely empty article is dropped without a second (single) API call."""
    store = {
        "kb_knowledge": [_article("e1", KNOWN_ERROR), _article("e2", KNOWN_ERROR)],
        KNOWN_ERROR: [_ext_row("e1", kb_cause="Has content")],  # e2 has no row
    }
    client = FakeClient(store)

    documents = scraper.collect_servicenow_kb_documents(client, "https://dev.service-now.com")

    # Only the bulk call — e2 stays empty and is dropped, no single fallback.
    assert _ext_calls(client.call_log) == [(KNOWN_ERROR, "bulk")]
    assert [d.metadata["sys_id"] for d in documents] == ["e1"]


def test_bulk_failure_falls_back_to_per_article_single_fetch():
    """When the bulk query raises, each article in that class is fetched singly."""
    store = {
        "kb_knowledge": [_article("f1", KNOWN_ERROR), _article("f2", KNOWN_ERROR)],
        KNOWN_ERROR: [_ext_row("f1", kb_cause="C1"), _ext_row("f2", kb_cause="C2")],
    }
    client = FakeClient(store, fail_bulk_classes={KNOWN_ERROR})

    documents = scraper.collect_servicenow_kb_documents(client, "https://dev.service-now.com")

    calls = _ext_calls(client.call_log)
    assert (KNOWN_ERROR, "bulk") in calls
    assert calls.count((KNOWN_ERROR, "single")) == 2  # one fallback per article
    bodies = _bodies_by_sys_id(documents)
    assert "C1" in bodies["f1"] and "C2" in bodies["f2"]


def test_bulk_fetch_chunks_large_id_lists():
    """sys_id lists longer than chunk_size produce multiple bulk queries."""
    store = {
        "kb_knowledge": [_article(f"a{i}", KNOWN_ERROR) for i in range(5)],
        KNOWN_ERROR: [_ext_row(f"a{i}", kb_cause=f"cause-{i}") for i in range(5)],
    }
    client = FakeClient(store)

    result = scraper.fetch_servicenow_kb_extension_content_bulk(
        client, KNOWN_ERROR, [f"a{i}" for i in range(5)], chunk_size=2
    )

    # 5 ids / chunk_size 2 -> 3 bulk queries.
    assert client.call_log.count((KNOWN_ERROR, "bulk")) == 3
    assert len(result) == 5
    assert "cause-0" in result["a0"] and "cause-4" in result["a4"]


def test_invalid_sys_class_name_is_not_queried():
    """A malformed sys_class_name is rejected before any API call."""
    client = FakeClient({})

    assert scraper.fetch_servicenow_kb_extension_content_bulk(client, "kb_knowledge", ["x"]) == {}
    assert scraper.fetch_servicenow_kb_extension_content_bulk(client, "bad-name; drop", ["x"]) == {}
    assert client.call_log == []
