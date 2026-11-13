import runpy
import sys
import types
import unittest
from pathlib import Path
from unittest.mock import patch


SCRIPT = Path(__file__).resolve().parents[3] / "scripts/instance_fonts.py"


class Axis:
    def __init__(self, tag, minimum, maximum):
        self.axisTag, self.minValue, self.maxValue = tag, minimum, maximum


class Font:
    def __init__(self, axes=None, glyphs=None, cmap=None):
        self.axes = axes if axes is not None else [Axis("wght", 100, 900)]
        self.glyphs = glyphs if glyphs is not None else [".notdef", "a"]
        table = types.SimpleNamespace(platformID=3, platEncID=1, language=0, format=4,
                                      cmap=cmap if cmap is not None else {97: "a"})
        self.tables = [table]
        self.saved = False

    def __getitem__(self, key):
        return types.SimpleNamespace(axes=self.axes) if key == "fvar" else types.SimpleNamespace(tables=self.tables)

    def getGlyphOrder(self):
        return self.glyphs

    def save(self, path):
        self.saved = True

    def close(self):
        pass


class InstanceTests(unittest.TestCase):
    def run_helper(self, source, result):
        ttlib = types.ModuleType("fontTools.ttLib")
        ttlib.TTFont = lambda path: source
        instancer = types.ModuleType("fontTools.varLib.instancer")
        instancer.AxisTriple = lambda *args: args
        instancer.instantiateVariableFont = lambda font, limits: result
        modules = {"fontTools": types.ModuleType("fontTools"), "fontTools.ttLib": ttlib,
                   "fontTools.varLib": types.ModuleType("fontTools.varLib"),
                   "fontTools.varLib.instancer": instancer}
        with patch.dict(sys.modules, modules):
            runpy.run_path(str(SCRIPT))

    def test_rejects_changed_glyph_count(self):
        result = Font([Axis("wght", 400, 700)], [".notdef"])
        with self.assertRaises(ValueError):
            self.run_helper(Font(), result)
        self.assertFalse(result.saved)

    def test_rejects_changed_cmap(self):
        result = Font([Axis("wght", 400, 700)], cmap={97: "other"})
        with self.assertRaises(ValueError):
            self.run_helper(Font(), result)
        self.assertFalse(result.saved)

    def test_rejects_extra_axis(self):
        result = Font([Axis("wght", 400, 700), Axis("wdth", 62, 100)])
        with self.assertRaises(ValueError):
            self.run_helper(Font(), result)
        self.assertFalse(result.saved)

    def test_rejects_wrong_weight_range(self):
        result = Font([Axis("wght", 400, 500)])
        with self.assertRaises(ValueError):
            self.run_helper(Font(), result)
        self.assertFalse(result.saved)

    def test_valid_instance_is_saved(self):
        result = Font([Axis("wght", 400, 700)])
        self.run_helper(Font(), result)
        self.assertTrue(result.saved)

    def test_already_instanced_is_accepted_without_save(self):
        source = Font([Axis("wght", 400, 700)])
        self.run_helper(source, Font())
        self.assertFalse(source.saved)


if __name__ == "__main__":
    unittest.main()
