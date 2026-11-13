#!/usr/bin/env python3
"""Parametric case for the gostt-guess handheld.

    pip install cadquery
    python3 case.py            # writes STEP and STL files to ./out

Everything is in millimetres. Z runs from the front face (0) to the back
and Y from bottom to top. Positions in the layout section are given as
(distance from the left edge as you look at the front, height); at() turns
them into model coordinates, where X runs the other way.

Every part size below came from a seller's listing or the maker's 3D model,
not from a part on a bench. Measure your parts with calipers, correct the
numbers in PARTS, and run this again before printing the full case. Print
fit_test first: it is the front face only, a 20 minute print.
"""
import os
import cadquery as cq

# ---------------------------------------------------------------- parts ---
PARTS = dict(
    # Waveshare 2.8inch Capacitive Touch LCD, used in landscape.
    display_w=73.06, display_h=50.54,   # module (cover glass) outline
    display_t=8.0,                      # glass face to the back of the connector (assumed)
    view_w=57.6, view_h=43.2,           # active area
    view_dx=0.0, view_dy=0.0,           # active area centre relative to module centre (assumed centred)
    # Adafruit 3351 enclosed speaker.
    spk_w=70.0, spk_h=30.0, spk_t=17.0,
    # Adafruit 6049 / 3421 I2S microphone breakout.
    mic_w=12.7, mic_h=16.7,
    # Raspberry Pi Zero 2 W on a PiSugar 3: the stack, battery side down.
    stack_w=65.0, stack_h=30.5,
    stack_t=19.0,                       # battery underside to the top of the Pi's board (from PiSugar's "xl" shell model)
    pi_parts=3.3,                       # tallest part above the Pi's board
    # Cherry MX style key switch.
    key_hole=14.0, key_plate=1.5,
    led_d=5.1,
)
FIT = 0.3          # clearance added around every pocket
WALL = 2.4
W, H = 150.0, 96.0
FOAM = 1.0         # foam tape under the battery
# Depth: face, display, room for wires, the Pi stack, foam, back.
D = round(WALL + PARTS["display_t"] + 7.2 + PARTS["stack_t"] + FOAM + WALL)
R = 8.0            # corner radius
SCREW_HOLE = 2.8   # an M3 screw cuts its own thread in this
SCREW_PASS = 3.4

# --------------------------------------------------------------- layout ---
def at(left, y):
    return (W - left, y)


DISPLAY = at(47.0, 65.3)               # centre of the display module
SPEAKER = at(46.0, 20.0)               # centre of the speaker
KEYS = {"yes": at(107, 48), "no": at(131, 48), "not_sure": at(107, 24), "play": at(131, 24)}
POWER = at(104, 80)
LED = at(120.5, 84)
MIC = at(137, 74)
STACK = at(46.5, 75.0)                 # centre of the Pi stack, which sits on the back cover
CORNERS = [(5.5, 5.5), (W - 5.5, 5.5), (5.5, H - 5.5), (W - 5.5, H - 5.5)]
SPK_POSTS = [(SPEAKER[0] + 41.5, SPEAKER[1]), (SPEAKER[0] - 41.5, SPEAKER[1])]
INNER_Z = D - WALL                     # inside face of the back cover


def rounded(w, h, depth, r):
    return cq.Workplane("XY").box(w, h, depth, centered=(False, False, False)).edges("|Z").fillet(r)


def box_at(cx, cy, w, h, z0, z1):
    return cq.Workplane("XY").box(w, h, z1 - z0, centered=(True, True, False)).translate((cx, cy, z0))


def cyl_at(cx, cy, d, z0, z1):
    return cq.Workplane("XY").circle(d / 2).extrude(z1 - z0).translate((cx, cy, z0))


def frame(cx, cy, w, h, z0, z1, t=1.6, gap=0.6):
    """Corner ribs that locate a part of size w x h, leaving the middle of
    each side open for cables and fingers."""
    outer = box_at(cx, cy, w + 2 * (FIT + t), h + 2 * (FIT + t), z0, z1)
    inner = box_at(cx, cy, w + 2 * FIT, h + 2 * FIT, z0 - 1, z1 + 1)
    ribs = outer.cut(inner)
    ribs = ribs.cut(box_at(cx, cy, w * gap, h + 20, z0 - 1, z1 + 1))
    ribs = ribs.cut(box_at(cx, cy, w + 20, h * gap, z0 - 1, z1 + 1))
    return ribs


def front():
    p = PARTS
    body = rounded(W, H, INNER_Z, R)
    body = body.faces("<Z").edges().chamfer(1.0)
    cavity = rounded(W - 2 * WALL, H - 2 * WALL, INNER_Z, R - WALL).translate((WALL, WALL, WALL))
    body = body.cut(cavity)

    # Posts for the back cover screws, and for the speaker clamp.
    for x, y in CORNERS:
        body = body.union(cyl_at(x, y, 8, WALL - 0.1, INNER_Z - 2.2))
    spk_back = WALL + p["spk_t"]
    for x, y in SPK_POSTS:
        body = body.union(cyl_at(x, y, 7, WALL - 0.1, spk_back))

    # Locating ribs.
    body = body.union(frame(*DISPLAY, p["display_w"], p["display_h"], WALL - 0.1, WALL + 3))
    body = body.union(frame(*SPEAKER, p["spk_w"], p["spk_h"], WALL - 0.1, WALL + 4))
    body = body.union(frame(*MIC, p["mic_w"], p["mic_h"], WALL - 0.1, WALL + 2, t=1.2, gap=0.4))

    # Keep the inside of the shell only: ribs and posts must not poke out.
    body = body.intersect(rounded(W, H, INNER_Z, R))

    # Screen window, with a bevel so the edge does not shade the picture.
    vx, vy = DISPLAY[0] + p["view_dx"], DISPLAY[1] + p["view_dy"]
    window = (cq.Workplane("XY").rect(p["view_w"] + 5, p["view_h"] + 5)
              .workplane(offset=WALL).rect(p["view_w"] + 1, p["view_h"] + 1).loft()
              .translate((vx, vy, 0)))
    body = body.cut(window)

    # Speaker grille: slots.
    for i in range(-9, 10):
        body = body.cut(box_at(SPEAKER[0] + i * 3.4, SPEAKER[1], 1.7, p["spk_h"] - 8, -1, WALL + 1))

    # Key switches snap into a plate 1.5 mm thick, so the face is thinned
    # from the inside around each one.
    for x, y in list(KEYS.values()) + [POWER]:
        body = body.cut(box_at(x, y, p["key_hole"], p["key_hole"], -1, WALL + 1))
        body = body.cut(box_at(x, y, 17, 17, p["key_plate"], WALL + 1))

    body = body.cut(cyl_at(*LED, p["led_d"], -1, WALL + 1))
    body = body.cut(cyl_at(*MIC, 2.0, -1, WALL + 1))

    # Screw holes.
    for x, y in CORNERS:
        body = body.cut(cyl_at(x, y, SCREW_HOLE, INNER_Z - 14, INNER_Z + 1))
    for x, y in SPK_POSTS:
        body = body.cut(cyl_at(x, y, SCREW_HOLE, spk_back - 10, spk_back + 1))

    # Opening in the top edge for the charging port, the battery board's own
    # buttons and lights, and the Pi's ports.
    pi_top = INNER_Z - FOAM - p["stack_t"]
    body = body.cut(box_at(STACK[0], H, p["stack_w"] - 6, 3 * WALL, pi_top - p["pi_parts"] - 0.5, pi_top + 9.5))
    return body


def back():
    p = PARTS
    plate = rounded(W, H, WALL, R).translate((0, 0, INNER_Z))
    plate = plate.faces(">Z").edges().chamfer(1.0)
    # A rim that drops inside the walls and keeps the cover from sliding.
    rim = rounded(W - 2 * WALL - 2 * FIT, H - 2 * WALL - 2 * FIT, 2.0, R - WALL).translate((WALL + FIT, WALL + FIT, INNER_Z - 2.0))
    rim = rim.cut(rounded(W - 2 * WALL - 2 * FIT - 3.2, H - 2 * WALL - 2 * FIT - 3.2, 4, R - WALL - 1.6)
                  .translate((WALL + FIT + 1.6, WALL + FIT + 1.6, INNER_Z - 3)))
    for x, y in CORNERS:
        rim = rim.cut(cyl_at(x, y, 9.5, INNER_Z - 3, INNER_Z + 1))
    # No rim where the port opening is.
    rim = rim.cut(box_at(STACK[0], H, p["stack_w"], 4 * WALL + 2, INNER_Z - 3, INNER_Z + 1))
    part = plate.union(rim)

    # Cradle for the Pi stack: walls on three sides, open towards the ports.
    cradle = frame(*STACK, p["stack_w"], p["stack_h"], INNER_Z - 7, INNER_Z + 0.1, t=2.0, gap=0.5)
    cradle = cradle.cut(box_at(STACK[0], STACK[1] + p["stack_h"] / 2 + 1, p["stack_w"] + 10, 6, INNER_Z - 8, INNER_Z))
    part = part.union(cradle)
    # Two bridges for a hook-and-loop strap that runs over the Pi lengthwise.
    for side in (-1, 1):
        x = STACK[0] + side * (p["stack_w"] / 2 + 5.5)
        bridge = box_at(x, STACK[1], 4, 20, INNER_Z - 5, INNER_Z + 0.1)
        bridge = bridge.cut(box_at(x, STACK[1], 6, 14, INNER_Z - 3, INNER_Z + 0.1))
        part = part.union(bridge)

    for x, y in CORNERS:
        part = part.cut(cyl_at(x, y, SCREW_PASS, INNER_Z - 3, D + 1))
        part = part.cut(cyl_at(x, y, 6.4, D - 1.2, D + 1))   # the screw head sits just below the surface
    return part


def clamp():
    """Bar that holds the speaker in its pocket."""
    span = abs(SPK_POSTS[1][0] - SPK_POSTS[0][0])
    bar = rounded(span + 8, 8, 3, 3.9).translate((-4, -4, 0))
    for x in (0, span):
        bar = bar.cut(cyl_at(x, 0, SCREW_PASS, -1, 4))
    return bar


def keycap(symbol, size=20.0, height=7.5):
    """A cap for an MX style switch, printed top face down."""
    cap = cq.Workplane("XY").rect(size, size).workplane(offset=height).rect(size - 3, size - 3).loft()
    cap = cap.edges("not(<Z or >Z)").fillet(1.5)
    hollow = (cq.Workplane("XY").rect(size - 3, size - 3).workplane(offset=height - 1.6).rect(size - 6, size - 6).loft()
              .translate((0, 0, -0.01)))
    cap = cap.cut(hollow)
    stem = cyl_at(0, 0, 5.6, 0.3, height - 1.5)
    cross = box_at(0, 0, 4.2, 1.4, 0, 4.3).union(box_at(0, 0, 1.4, 4.2, 0, 4.3))
    cap = cap.union(stem.cut(cross))
    top = height
    mark = None
    if symbol == "yes":       # tick
        mark = cq.Workplane("XY").polyline([(-5, 0), (-2, -4), (5.5, 4), (4, 5), (-2, -1.2), (-3.8, 1.2)]).close().extrude(0.8)
    elif symbol == "no":      # cross
        mark = box_at(0, 0, 12, 2.2, 0, 0.8).rotate((0, 0, 0), (0, 0, 1), 45).union(
            box_at(0, 0, 12, 2.2, 0, 0.8).rotate((0, 0, 0), (0, 0, 1), -45))
    elif symbol == "not_sure":  # question mark, from simple shapes
        arc = (cq.Workplane("XY").circle(3.6).circle(1.6).extrude(0.8)
               .cut(box_at(-3, -1.5, 6, 5, -1, 2)).translate((0, 2.2, 0)))
        mark = arc.union(box_at(0, -2.0, 2.0, 3.0, 0, 0.8)).union(cyl_at(0, -5.2, 2.2, 0, 0.8))
    elif symbol == "play":    # triangle
        mark = cq.Workplane("XY").polyline([(-3.5, -5), (5, 0), (-3.5, 5)]).close().extrude(0.8)
    elif symbol == "power":   # ring and bar
        ringm = cq.Workplane("XY").circle(4.6).circle(3.0).extrude(0.8).cut(box_at(0, 4, 4.4, 5, -1, 2))
        mark = ringm.union(box_at(0, 3.2, 1.7, 5.5, 0, 0.8))
    if mark is not None:
        cap = cap.cut(mark.translate((0, 0, top - 0.6)))
    return cap


def fit_test():
    """The first few millimetres of the front: every opening and pocket,
    in a quick print, to try the parts in before printing the real thing."""
    return front().intersect(cq.Workplane("XY").box(W + 40, H + 40, 14, centered=(True, True, False)).translate((W / 2, H / 2, -5)).cut(
        cq.Workplane("XY").box(W + 40, H + 40, 20, centered=(True, True, False)).translate((W / 2, H / 2, WALL + 3.2))))


def build():
    parts = {
        "front": front(), "back": back(), "speaker_clamp": clamp(), "fit_test": fit_test(),
        "keycap_power": keycap("power", size=18.0, height=6.0),
    }
    for name in KEYS:
        parts["keycap_" + name] = keycap(name)
    return parts


if __name__ == "__main__":
    out = os.path.join(os.path.dirname(os.path.abspath(__file__)), "out")
    os.makedirs(out, exist_ok=True)
    print(f"case {W:.0f} x {H:.0f} x {D:.0f} mm")
    for name, part in build().items():
        solid = part.val()
        bb = solid.BoundingBox()
        ok = solid.isValid() and len(part.solids().vals()) == 1
        print(f"  {name:16s} {bb.xlen:6.1f} x {bb.ylen:5.1f} x {bb.zlen:5.1f}  {solid.Volume() / 1000:6.1f} cm3  {'ok' if ok else 'CHECK: not one valid solid'}")
        cq.exporters.export(part, os.path.join(out, name + ".stl"), tolerance=0.05, angularTolerance=0.2)
        cq.exporters.export(part, os.path.join(out, name + ".step"))
