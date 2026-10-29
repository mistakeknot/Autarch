import os

import pytest

from tools.rulingjudge import store as st


@pytest.fixture
def use_root(monkeypatch):
    """Test-only injection point: make a directory the permitted root. The production function reads
    nothing from the environment, so the CLI under test has no switch of its own."""
    def use(path):
        monkeypatch.setattr(st, "permitted_root", lambda: os.path.realpath(str(path)))
    return use


@pytest.fixture(autouse=True)
def state_root(tmp_path, use_root):
    """Judge state may only live under the permitted root. Tests use their own temp dir as that root;
    tests of the refusal itself substitute another one."""
    use_root(tmp_path)
