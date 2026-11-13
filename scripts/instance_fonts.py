import hashlib
import json
from pathlib import Path

from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import AxisTriple, instantiateVariableFont


FINGERPRINTS = {
    "be-vietnam-pro-var": (512, "d8e1e0da54a1438b47a620c9796e290464e52758b802dc06c4c3b730693f4546"),
    "noto-serif-var": (1125, "2926c74ba2c1ea1691967d4e3e38d4ab0896aee1dbc2ac22e55c8ea9490645e0"),
    "roboto-mono-var": (600, "8c87fd88df92527a386b2258348bbf3dc2e6e6e0f5a77b93c99f180064955563"),
}


def cmap_entries(font):
    return [
        (table.platformID, table.platEncID, table.language, table.format, table.cmap,
         getattr(table, "uvsDict", None))
        for table in font["cmap"].tables
    ]


def verify(name, source, result):
    if len(source.getGlyphOrder()) != len(result.getGlyphOrder()):
        raise ValueError("instancing changed glyph count")
    if cmap_entries(source) != cmap_entries(result):
        raise ValueError("instancing changed cmap")
    count, expected_hash = FINGERPRINTS[name]
    cmap = json.dumps(sorted(result.getBestCmap().items())).encode("utf-8")
    if len(result.getGlyphOrder()) != count or hashlib.sha256(cmap).hexdigest() != expected_hash:
        raise ValueError(f"unexpected glyph fingerprint: {name}")
    axes = [(axis.axisTag, axis.minValue, axis.maxValue) for axis in result["fvar"].axes]
    if axes != [("wght", 400, 700)]:
        raise ValueError(f"unexpected instance axes: {axes}")


def main():
    font_dir = Path(__file__).resolve().parent.parent / "apps/web/public/fonts"
    for name in FINGERPRINTS:
        path = font_dir / f"{name}.woff2"
        font = TTFont(path)
        axes = {axis.axisTag: axis for axis in font["fvar"].axes}
        weight = axes["wght"]
        if weight.minValue == 400 and weight.maxValue == 700 and len(axes) == 1:
            verify(name, font, font)
            font.close()
            continue
        limits = {"wght": AxisTriple(400, None, 700)}
        if name == "noto-serif-var" and "wdth" in axes:
            limits["wdth"] = 100
        instance = instantiateVariableFont(font, limits)
        verify(name, font, instance)
        instance.flavor = "woff2"
        instance.save(path)
        instance.close()
        font.close()


if __name__ == "__main__":
    main()
