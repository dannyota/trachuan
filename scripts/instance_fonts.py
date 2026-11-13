import hashlib
from pathlib import Path

from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import AxisTriple, instantiateVariableFont


FINGERPRINTS = {
    "be-vietnam-pro-var": (
        "fc482bed0759a243e2ca6c4b3250764035f0bfc33fdc13be4743fac6449d9be6",
        "c4015fa5381f1ac39c6587caa7aaf3aac0689fee50f3dd5587df170a119bd59c",
    ),
    "noto-serif-var": (
        "6c3653a038615ce69fe0ce45db29d06466582dd02301f41058344fdaeb5d1d1b",
        "1bf7e8832036214e8410081c4cac7700f532e8fb3e8b2ae0db33485e391fb476",
    ),
    "roboto-mono-var": (
        "c471a1dadd3a7ef76d9d775744e46ff6f5f3683f37d23797704bd194c8214c8f",
        "21ab3f1032b6d2f6cd343ec73ef58290dd7fb071ba7e24e41ab3ff88aef775fa",
    ),
}


def fingerprint(font):
    cmap = font["cmap"].compile(font)
    glyphs = "\n".join(font.getGlyphOrder()).encode("utf-8")
    return hashlib.sha256(cmap).hexdigest(), hashlib.sha256(glyphs).hexdigest()


def verify(name, source, result):
    result_fingerprint = fingerprint(result)
    if fingerprint(source) != result_fingerprint:
        raise ValueError("instancing changed cmap or glyph order")
    if result_fingerprint != FINGERPRINTS[name]:
        raise ValueError(f"unexpected font fingerprint: {name}")
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
