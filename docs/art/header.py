# Builds docs/header.png from docs/art/hermit-crab.png: the crab on the left,
# the name and what Hermit mods on the right, on the art's own black.
# Run from the repository root: python3 docs/art/header.py docs/art/hermit-crab.png docs/header.png
import sys
from PIL import Image, ImageDraw, ImageFont

art_path, out_path = sys.argv[1], sys.argv[2]
W, H = 1800, 560
TEAL = (67, 181, 164)
WHITE = (240, 240, 242)
GREY = (161, 161, 170)  # zinc-400
# Beyond these columns the art is black, at most 1 in 255.
CROP = (118, 0, 1462, 1024)
DIM = (82, 82, 91)      # zinc-600

art = Image.open(art_path).convert("RGB")
# The art's glow fades to black inside the picture, reaching down to its
# 950th row, so it is used to its full height and cut only at the sides,
# where it is black already: cutting into the glow leaves a corner in it.
crab = art.crop(CROP)
scale = H / crab.height
crab = crab.resize((round(crab.width * scale), round(crab.height * scale)), Image.LANCZOS)

banner = Image.new("RGB", (W, H), (0, 0, 0))
banner.paste(crab, (0, 0))

d = ImageDraw.Draw(banner)
fonts = "/usr/share/fonts/noto/"
title = ImageFont.truetype(fonts + "NotoSans-Black.ttf", 168)
lead = ImageFont.truetype(fonts + "NotoSans-Medium.ttf", 44)
small = ImageFont.truetype(fonts + "NotoSans-Bold.ttf", 27)

x = crab.width
d.text((x - 8, 82), "Hermit", font=title, fill=WHITE)
d.text((x, 308), "A mod manager for Linux", font=lead, fill=GREY)

# What it mods, as chips.
labels = ["BepInEx", "Creation Engine", "REDengine", "Unreal Engine"]
pad, gap = 16, 12
widths = [d.textlength(label, font=small) + 2 * pad for label in labels]
assert x + sum(widths) + gap * (len(labels) - 1) <= W - 48, "the chips do not fit"
cx, cy = x, 392
for label, w in zip(labels, widths):
    d.rounded_rectangle((cx, cy, cx + w, cy + 50), radius=25, outline=TEAL, width=3)
    d.text((cx + pad, cy + 7), label, font=small, fill=TEAL)
    cx += w + gap

# Rounded corners, so the banner sits as a card on a light page too.
mask = Image.new("L", (W, H), 0)
ImageDraw.Draw(mask).rounded_rectangle((0, 0, W - 1, H - 1), radius=32, fill=255)
out = banner.convert("RGBA")
out.putalpha(mask)
out.save(out_path, optimize=True)
