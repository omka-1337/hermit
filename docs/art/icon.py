# Builds Hermit's icons from docs/art/hermit-crab.png: the crab on a rounded
# square of the art's own black. Run from the repository root:
#
#	python3 docs/art/icon.py docs/art/hermit-crab.png
#
# and then, for the Windows and macOS icons, in build/:
#
#	wails3 generate icons -input appicon.png -macfilename darwin/icons.icns -windowsfilename windows/icon.ico
import sys
from PIL import Image, ImageDraw

art = Image.open(sys.argv[1]).convert("RGB")
SIZE = 1024
# The art's glow fades to black inside the picture, so the crab is taken
# with all of it, cut only at the sides where the art is black already, at
# most 1 in 255: cutting into the glow leaves a corner in it.
crab = art.crop((118, 0, 1462, 1024))
scale = SIZE / crab.width
crab = crab.resize((SIZE, round(crab.height * scale)), Image.LANCZOS)
square = Image.new("RGB", (SIZE, SIZE), (0, 0, 0))
square.paste(crab, (0, (SIZE - crab.height) // 2 + 20))

mask = Image.new("L", (SIZE, SIZE), 0)
ImageDraw.Draw(mask).rounded_rectangle((0, 0, SIZE - 1, SIZE - 1), radius=round(SIZE * 0.22), fill=255)
icon = square.convert("RGBA")
icon.putalpha(mask)

for path, size in [
    ("build/appicon.png", 1024),
    ("internal/app/icon.png", 512),
    ("frontend/public/favicon.png", 256),
]:
    icon.resize((size, size), Image.LANCZOS).save(path, optimize=True)

# The Hermit menu in the title bar shows the crab at 20 pixels, where a black
# tile would swallow it: the crab alone, the art's black made transparent so
# that its outlines become the title bar's own dark, cropped close.
gray = art.convert("L")
box = gray.point(lambda v: 255 if v > 40 else 0).getbbox()
side = max(box[2] - box[0], box[3] - box[1]) + 16
cx, cy = (box[0] + box[2]) // 2, (box[1] + box[3]) // 2
small = Image.new("RGB", (side, side), (0, 0, 0))
small.paste(art.crop((cx - side // 2, cy - side // 2, cx + side // 2, cy + side // 2)))
alpha = small.convert("L").point(lambda v: max(0, min(255, (v - 14) * 6)))
small = small.convert("RGBA")
small.putalpha(alpha)
# Sharp up to three times the size it is shown at.
small.resize((64, 64), Image.LANCZOS).save("frontend/src/assets/hermit.png", optimize=True)
