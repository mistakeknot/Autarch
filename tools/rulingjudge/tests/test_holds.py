"""Hold tests: one per class. Asks are synthetic."""

from datetime import datetime, timezone

import pytest

from tools.rulingjudge import holds
from tools.rulingjudge.ask import Ask, Option, parse_ask

T = datetime(2026, 1, 1, tzinfo=timezone.utc)


def ask(question="Pick a colour for the chart", options=None, **kw):
    options = options or (Option("a", "ruling-only", "Blue"), Option("b", "ruling-only", "Green"))
    base = dict(
        decision_id="d", revision="1", question=question, subject="", project="p", options=tuple(options),
        recommendation=None, delegable=True, root_run=False, move=False, filed_at=T,
    )
    base.update(kw)
    return Ask(**base)


def test_clean_ask_is_not_held():
    assert not holds.evaluate(ask()).held


def test_not_delegable():
    assert holds.evaluate(ask(delegable=False)).reasons == ("not-delegable",)


def test_approval_option():
    opts = (Option("a", "instruction", "Go", approval={"kind": "merge"}), Option("b", "ruling-only", "No"))
    assert "approval-option" in holds.evaluate(ask(options=opts)).reasons


def test_explicit_irreversible_holds_but_absent_does_not():
    assert "irreversible-option" in holds.evaluate(ask(options=(Option("a", "x", "A", reversible=False), Option("b", "x", "B")))).reasons
    assert not holds.evaluate(ask(options=(Option("a", "x", "A", reversible=None), Option("b", "x", "B", reversible=True)))).held


def test_root_run_and_move():
    assert holds.evaluate(ask(root_run=True)).reasons == ("root-run",)
    assert holds.evaluate(ask(move=True)).reasons == ("move-block",)


@pytest.mark.parametrize(
    "gate,text",
    [
        ("spend", "This will spend $20 on credits"),
        ("credential", "Rotate the API key"),
        ("publication", "Publish the release to npm"),
        ("production_root", "Deploy to production after the install"),
        ("permission", "Edit settings.json to add an allow line"),
        ("privacy_destination", "Upload the data to a third-party service"),
        ("destructive", "Delete the old branch and force-push"),
        ("signing_authority", "Change the signing certificate"),
    ],
)
def test_each_gate_class(gate, text):
    assert f"gate:{gate}" in holds.evaluate(ask(question=text)).reasons


def test_gate_matches_option_label_and_instruction():
    opts = (Option("a", "instruction", "Fine", instruction="then publish it"), Option("b", "ruling-only", "No"))
    assert "gate:publication" in holds.evaluate(ask(options=opts)).reasons
    opts = (Option("a", "ruling-only", "Delete everything"), Option("b", "ruling-only", "No"))
    assert "gate:destructive" in holds.evaluate(ask(options=opts)).reasons


def test_gate_words_need_word_boundaries():
    assert not holds.evaluate(ask(question="Use the tagline and the secretary view")).held


def test_truncated_question_and_option():
    assert "truncated-question" in holds.evaluate(ask(question="x" * (holds.MAX_QUESTION + 1))).reasons
    opts = (Option("a", "instruction", "A", instruction="y" * (holds.MAX_OPTION + 1)), Option("b", "x", "B"))
    assert "truncated-option" in holds.evaluate(ask(options=opts)).reasons


def test_every_gate_class_has_a_test_row():
    tested = {"spend", "credential", "publication", "production_root", "permission", "privacy_destination", "destructive", "signing_authority"}
    assert set(holds.GATES) == tested


def test_gate_table_hash_is_stable_hex():
    assert holds.gate_table_hash() == holds.gate_table_hash() and len(holds.gate_table_hash()) == 64


def test_parse_ask_reads_semantics_and_markers():
    body = '{"question":"Run it?\\n```root-run\\nscript: x","options":[{"id":"a","kind":"instruction","label":"Go","instruction":"do","reversible":true},{"id":"b","kind":"ruling-only","label":"No"}],"recommendation":"a","subject":"s","project":"p"}'
    a = parse_ask("d1", "r1", body, "2026-01-02T03:04:05Z")
    assert a.root_run and not a.move
    assert a.options[0].reversible is True and a.options[1].reversible is None
    assert a.recommendation == "a" and len(a.body_sha256) == 64
    assert parse_ask("d1", "r1", "not json", "2026-01-02T03:04:05Z") is None
    assert parse_ask("d1", "r1", '{"options":[]}', "2026-01-02T03:04:05Z") is None
