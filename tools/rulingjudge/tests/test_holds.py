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


@pytest.mark.parametrize("value", [True, False, None])
def test_reversible_never_holds_whatever_its_value(value):
    h = holds.evaluate(ask(options=(Option("a", "x", "A", reversible=value), Option("b", "x", "B", reversible=value))))
    assert not h.held and "irreversible-option" not in h.reasons


def test_one_explicitly_irreversible_option_among_others_does_not_hold():
    opts = (Option("a", "x", "A", reversible=False), Option("b", "x", "B", reversible=True), Option("c", "x", "C"))
    h = holds.evaluate(ask(options=opts))
    assert not h.held and h.reasons == ()


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


def test_reversible_true_or_absent_neither_holds_nor_permits_in_the_real_go_shape():
    """`reversible` is a Go bool with omitempty, so a false value never appears in the stored body
    and an absent one means false or unset. A true value is not permission either."""
    import json
    from tools.rulingjudge.ask import parse_ask

    def parsed(opts, question="Pick a colour for the chart"):
        body = json.dumps({"question": question, "subject": "s", "project": "p", "options": opts})
        return parse_ask("d1", "1", body, "2026-02-01T00:00:00Z", 1)

    plain = [{"id": "a", "kind": "ruling-only", "label": "A"}, {"id": "b", "kind": "ruling-only", "label": "B"}]
    marked = [{**plain[0], "reversible": True}, plain[1]]
    assert not holds.evaluate(parsed(plain)).held and not holds.evaluate(parsed(marked)).held
    # true does not lift a hold that something else raised
    risky = parsed(marked, question="Deploy to production after the install")
    assert holds.evaluate(risky).held
    # and an approval-carrying option still holds with reversible true
    approval = [{**marked[0], "approval": {"kind": "merge"}}, plain[1]]
    assert "approval-option" in holds.evaluate(parsed(approval)).reasons
