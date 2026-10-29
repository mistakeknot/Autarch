"""Family tests. Card keys and ids are synthetic."""

from datetime import datetime, timedelta, timezone

from tools.rulingjudge.families import DecisionRef, build_families

T0 = datetime(2026, 1, 1, tzinfo=timezone.utc)


def d(i, hours, **kw):
    return DecisionRef(id=i, filed_at=T0 + timedelta(hours=hours), **kw)


def test_unrelated_decisions_are_separate_families():
    fam = build_families([d("a", 0, task_id="t1"), d("b", 1, task_id="t2")])
    assert fam.family_of["a"] != fam.family_of["b"]
    assert fam.is_first("a") and fam.is_first("b")


def test_same_task_different_generation_joins():
    fam = build_families([d("a", 0, task_id="t1"), d("b", 5, task_id="t1")])
    assert fam.family_of["a"] == fam.family_of["b"]
    assert fam.is_first("a") and not fam.is_first("b")
    assert fam.earlier_in_family("b") == ["a"]


def test_supersedes_joins():
    fam = build_families([d("a", 0), d("b", 1, supersedes="a")])
    assert fam.family_of["a"] == fam.family_of["b"]
    assert fam.links["supersedes"] == 1


def test_supersedes_pointing_outside_set_is_ignored():
    fam = build_families([d("b", 1, supersedes="missing")])
    assert fam.is_first("b")


def test_identity_and_semantic_key_join_blank_does_not():
    fam = build_families(
        [
            d("a", 0, identity="i1"),
            d("b", 1, identity="i1"),
            d("c", 2, semantic_key="s"),
            d("e", 3, semantic_key="s"),
            d("f", 4, semantic_key=""),
            d("g", 5, semantic_key=""),
        ]
    )
    assert fam.family_of["a"] == fam.family_of["b"]
    assert fam.family_of["c"] == fam.family_of["e"]
    assert fam.family_of["f"] != fam.family_of["g"]


def test_shared_request_identity_across_two_cards():
    """Shape of two cards that carry one request: different task ids and card keys."""
    a = d("a", 0, task_id="t-1", request_identity="req", card_keys=frozenset({"AAA-426"}))
    b = d("b", 3, task_id="t-2", request_identity="req", card_keys=frozenset({"AAA-431"}))
    fam = build_families([a, b])
    assert fam.family_of["a"] == fam.family_of["b"]
    assert fam.cards_of("a") == frozenset({"AAA-426", "AAA-431"})
    assert fam.cards_of("b") == fam.cards_of("a")
    assert not fam.is_first("b")


def test_transitive_chain_and_first_is_chronological_not_id_order():
    fam = build_families(
        [
            d("z", 0, identity="i"),
            d("m", 1, identity="i", supersedes="z"),
            d("a", 2, task_id="t", semantic_key="k"),
            d("b", 3, semantic_key="k", identity="i"),
        ]
    )
    assert len(set(fam.family_of.values())) == 1
    assert fam.members[fam.family_of["a"]] == ["z", "m", "a", "b"]
    assert fam.first[fam.family_of["a"]] == "z"
