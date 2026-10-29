"""Load the Clavain selector from the selected installation, unchanged. Fails closed."""

from __future__ import annotations

import importlib
import os
import sys
from dataclasses import dataclass
from types import ModuleType


class SelectorUnavailable(RuntimeError):
    """The selector could not be imported; nothing may be sent."""


@dataclass(frozen=True)
class Selector:
    root: str
    contract: ModuleType
    egress: ModuleType
    jev: ModuleType
    questions: ModuleType


def load(root: str | None = None) -> Selector:
    root = root or os.environ.get("CLAVAIN_SELECTED_ROOT")
    if not root:
        raise SelectorUnavailable("CLAVAIN_SELECTED_ROOT is not set; refusing to run without the selected Clavain selector")
    scripts = os.path.join(root, "scripts")
    if not os.path.isdir(os.path.join(scripts, "clavain_selector")):
        raise SelectorUnavailable(f"no clavain_selector package under {scripts}")
    if scripts not in sys.path:
        sys.path.insert(0, scripts)
    try:
        return Selector(
            root=root,
            contract=importlib.import_module("clavain_selector.contract"),
            egress=importlib.import_module("clavain_selector.egress"),
            jev=importlib.import_module("clavain_selector.jev_client"),
            questions=importlib.import_module("clavain_selector.questions"),
        )
    except Exception as exc:  # any import problem fails closed
        raise SelectorUnavailable(f"clavain_selector import failed: {type(exc).__name__}") from exc
