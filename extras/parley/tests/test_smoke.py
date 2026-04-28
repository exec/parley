"""Minimal smoke test — package imports and version is consistent."""
import parley


def test_version():
    assert parley.__version__ == "1.1.0"


def test_top_level_exports_intact():
    """Sanity check that the top-level surface still binds."""
    assert parley.Bot is not None
    assert parley.Selfbot is not None
    assert parley.CommandBot is not None
    assert parley.Cog is not None
