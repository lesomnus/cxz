`uniform-chip.woff` is an original synthetic test font derived from the Nox
fractional chip fixture. Every spacing glyph has the same advance. It catches
native textarea reservations that round down and leave the caret inside a chip.
It is a browser test fixture, never an application font.

Regenerate with `python generate-uniform-chip.py` with FontTools installed in a
development environment.
