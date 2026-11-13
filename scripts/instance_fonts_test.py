import hashlib
import json
import runpy
import sys
import types
import unittest
from pathlib import Path
from unittest.mock import patch


SCRIPT = Path(__file__).resolve().parent / "instance_fonts.py"


class Axis:
    def __init__(self, tag, minimum, maximum):
        self.axisTag, self.minValue, self.maxValue = tag, minimum, maximum


class Font:
    def __init__(self, axes=None, glyphs=None, cmap=None):
        self.axes = axes if axes is not None else [Axis("wght", 100, 900)]
        self.glyphs = glyphs if glyphs is not None else [".notdef", "a"]
        table = types.SimpleNamespace(platformID=3, platEncID=1, language=0, format=4,
                                      cmap=cmap if cmap is not None else {97: "a"})
        other = types.SimpleNamespace(platformID=0, platEncID=3, language=0, format=4,
                                      cmap={98: "b"})
        self.tables = [table, other]
        self.saved = False

    def __getitem__(self, key):
        if key == "fvar":
            return types.SimpleNamespace(axes=self.axes)
        return types.SimpleNamespace(compile=lambda font: json.dumps(
            [sorted(table.cmap.items()) for table in self.tables]
        ).encode("utf-8"))

    def getGlyphOrder(self):
        return self.glyphs

    def getBestCmap(self):
        return self.tables[0].cmap

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
            namespace = runpy.run_path(str(SCRIPT))
            main = namespace["main"]
            original = Font()
            cmap = hashlib.sha256(original["cmap"].compile(original)).hexdigest()
            glyphs = hashlib.sha256(b".notdef\na").hexdigest()
            main.__globals__["FINGERPRINTS"] = {"test-font": (cmap, glyphs)}
            main()

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

    def test_rejects_already_instanced_missing_glyph(self):
        source = Font([Axis("wght", 400, 700)], [".notdef"])
        with self.assertRaises(ValueError):
            self.run_helper(source, Font())

    def test_rejects_already_instanced_changed_cmap(self):
        source = Font([Axis("wght", 400, 700)], cmap={97: "other"})
        with self.assertRaises(ValueError):
            self.run_helper(source, Font())

    def test_rejects_already_instanced_changed_nonpreferred_cmap(self):
        source = Font([Axis("wght", 400, 700)])
        source.tables[1].cmap[98] = "other"
        with self.assertRaises(ValueError):
            self.run_helper(source, Font())

    def test_rejects_already_instanced_renamed_glyph(self):
        source = Font([Axis("wght", 400, 700)], [".notdef", "renamed"])
        with self.assertRaises(ValueError):
            self.run_helper(source, Font())

    def test_already_instanced_is_accepted_without_save(self):
        source = Font([Axis("wght", 400, 700)])
        self.run_helper(source, Font())
        self.assertFalse(source.saved)


if __name__ == "__main__":
    unittest.main()
