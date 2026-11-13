#!/usr/bin/env python3
"""Checks the case against the makers' own models of the parts.

    python3 check_fit.py <folder with 2423.step 3351.step ps3plus.step>

The models are not kept in this repository; the README says where each
comes from. Each part is placed where the case expects it and intersected
with every printed piece. Any overlap is a collision.
"""
import os
import sys
import cadquery as cq
import case as c

ref = sys.argv[1]
load = lambda n: cq.importers.importStep(os.path.join(ref, n))

placed = {}
# Display board: half a turn, board top at TFT_TOP. Its model carries the
# four small switches as a second body floating above the board; use a box
# of their documented height on the board instead.
tft = load("2423.step")
board = max(tft.solids().vals(), key=lambda s: s.Volume())
placed["display board"] = cq.Workplane(obj=board).rotate((0, 0, 0), (0, 0, 1), 180).translate((c.SX + c.TFT_W, c.SY, c.TFT_TOP))
x0, y1 = c.tft(6.0, -49.45)
x1, y0 = c.tft(1.0, -7.3)
placed["display board switches"] = c.box(x0, x1, y0, y1, c.TFT_TOP, c.TFT_TOP + 5.0)
# Speaker: sound side against the inside of the front.
placed["speaker"] = (load("3351.step").rotate((0, 0, 0), (1, 0, 0), 180)
                     .translate((c.SPEAKER[0], c.SPEAKER[1] + c.SPK_H, c.FACE_UNDER)))
# PiSugar 3 Plus: nuts against the underside of the Pi.
placed["PiSugar board"] = (load("ps3plus.step").rotate((0, 0, 0), (1, 0, 0), 90)
                           .translate((c.SX + 52.5, c.SY + 28.0, c.PI_BOTTOM - c.SUGAR_NUTS)))
bx0 = c.SX + 20.0
placed["battery space"] = c.box(bx0, bx0 + 65, c.SY, c.SY + 56, c.WALL + c.FOAM, c.WALL + c.FOAM + c.SUGAR_BATTERY)
# Pi 4: board, plus generous blocks for its connectors.
placed["Pi board"] = c.box(c.SX, c.SX + 85, c.SY, c.SY + 56, c.PI_BOTTOM, c.PI_TOP)
placed["Pi USB and Ethernet"] = c.box(c.SX - 3.2, c.SX + 18, c.SY + 1, c.SY + 55, c.PI_TOP, c.PI_TOP + 15.5)   # the display board is made to sit just above these
placed["Pi HDMI, power, audio"] = c.box(c.SX + 25, c.SX + 81, c.SY + 49, c.SY + 56 + 3.0, c.PI_TOP, c.PI_TOP + 6.5)
# Key switches: body and legs under the plate, cap above it.
for name, (x, y) in c.KEYS.items():
    top = c.T - c.DECK_DROP
    placed["switch " + name] = c.box(x - 7.8, x + 7.8, y - 7.8, y + 7.8, top - c.KEY_PLATE - 8.3, top - c.KEY_PLATE - 0.01)
    size, height = (18.0, 6.0) if name == "power" else (20.0, 7.5)
    cap_top = top + c.KEY_STEM_TOP - 4.0 + height
    placed["keycap " + name] = c.box(x - size / 2, x + size / 2, y - size / 2, y + size / 2, cap_top - height, cap_top)

printed = {"front": c.front(), "back": c.back(), "deck": c.deck(), "lid (closed)": c.lid()}
bad = 0
for pname, p in placed.items():
    for cname, body in printed.items():
        v = sum(s.Volume() for s in p.intersect(body).solids().vals())
        if v > 0.5:
            bad += 1
            bb = p.intersect(body).val().BoundingBox()
            print(f"COLLISION {pname} / {cname}: {v:.1f} mm3 at X[{bb.xmin:.1f},{bb.xmax:.1f}] Y[{bb.ymin:.1f},{bb.ymax:.1f}] Z[{bb.zmin:.1f},{bb.zmax:.1f}]")
names = list(placed)
for i, a in enumerate(names):
    for b in names[i + 1:]:
        if {a.split()[0], b.split()[0]} <= {"switch", "keycap"} or (a.startswith("display") and b.startswith("display")):
            continue
        if a.startswith("PiSugar") and b.startswith("battery"):
            continue   # the battery connector reaches into the battery's space by design
        v = sum(s.Volume() for s in placed[a].intersect(placed[b]).solids().vals())
        if v > 0.5:
            bad += 1
            print(f"COLLISION {a} / {b}: {v:.1f} mm3")

# The charging port must sit inside its opening.
sugar = placed["PiSugar board"].val().BoundingBox()
port_x = c.pi(c.SUGAR_USBC_X)
under = c.PI_BOTTOM - c.SUGAR_NUTS - c.SUGAR_PCB
print(f"PiSugar board top edge at Y {sugar.ymax:.2f}; inside of the top wall at Y {c.H - c.WALL:.2f}")
print(f"charging port: centre X {port_x:.2f}, Z {under - 3.2:.2f} to {under:.2f}; opening Z {under - 1.6 - 3.75:.2f} to {under - 1.6 + 3.75:.2f}")
cap_top = max(p.val().BoundingBox().zmax for n, p in placed.items() if n.startswith("keycap"))
print(f"keycaps stand {cap_top - c.T:.1f} mm above the front; the lid leaves {c.LID_RIM:.1f} mm")
print("no collisions" if not bad else f"{bad} collisions")
sys.exit(1 if bad else 0)
