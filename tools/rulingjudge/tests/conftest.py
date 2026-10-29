import pytest


@pytest.fixture(autouse=True)
def state_root(tmp_path, monkeypatch):
    """Judge state may only live under the permitted root. Tests use their own temp dir as that root;
    tests of the refusal itself point the variable somewhere else."""
    monkeypatch.setenv("RULINGJUDGE_STATE_ROOT", str(tmp_path))
