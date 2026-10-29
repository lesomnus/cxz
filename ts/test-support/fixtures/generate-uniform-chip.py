from pathlib import Path

from fontTools.ttLib import TTFont

# Reuse our original rectangle-glyph fixture, giving every spacing candidate
# the same advance to reproduce fixed-width fonts without relying on the OS.
root = Path(__file__).resolve().parents[2]
font = TTFont(root / "packages/nox/e2e/fixtures/fractional-chip.woff")
for name, (_, bearing) in font["hmtx"].metrics.items():
    font["hmtx"].metrics[name] = (550, bearing)
font.recalcTimestamp = False
font.save(Path(__file__).with_name("uniform-chip.woff"))
