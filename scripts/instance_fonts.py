from pathlib import Path

from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import AxisTriple, instantiateVariableFont


def cmap_entries(font):
    return [
        (table.platformID, table.platEncID, table.language, table.format, table.cmap,
         getattr(table, "uvsDict", None))
        for table in font["cmap"].tables
    ]


def verify(source, result):
    if len(source.getGlyphOrder()) != len(result.getGlyphOrder()):
        raise ValueError("instancing changed glyph count")
    if cmap_entries(source) != cmap_entries(result):
        raise ValueError("instancing changed cmap")
    axes = [(axis.axisTag, axis.minValue, axis.maxValue) for axis in result["fvar"].axes]
    if axes != [("wght", 400, 700)]:
        raise ValueError(f"unexpected instance axes: {axes}")


font_dir = Path(__file__).resolve().parent.parent / "apps/web/public/fonts"
for name in ("be-vietnam-pro-var", "noto-serif-var", "roboto-mono-var"):
    path = font_dir / f"{name}.woff2"
    font = TTFont(path)
    axes = {axis.axisTag: axis for axis in font["fvar"].axes}
    weight = axes["wght"]
    if weight.minValue == 400 and weight.maxValue == 700 and len(axes) == 1:
        verify(font, font)
        font.close()
        continue
    limits = {"wght": AxisTriple(400, None, 700)}
    if name == "noto-serif-var" and "wdth" in axes:
        limits["wdth"] = 100
    instance = instantiateVariableFont(font, limits)
    verify(font, instance)
    instance.flavor = "woff2"
    instance.save(path)
    instance.close()
    font.close()
