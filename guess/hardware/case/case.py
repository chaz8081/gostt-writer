#!/usr/bin/env python3
"""Parametric clamshell case for the gostt-guess handheld.

    pip install cadquery
    python3 case.py            # writes STEP and STL files to ./out

Millimetres throughout. X runs left to right and Y bottom to top as you
look at the front; Z runs from the back (0) towards you.

Where each number comes from is noted beside it:
  [cad]  measured from the maker's own CAD or board file (see README)
  [doc]  stated in the maker's documentation
  [std]  an industry-standard dimension (Cherry MX switch, 2.54 mm header)
  [est]  not available from a primary source; the design leaves slack for it
"""
import os
import cadquery as cq

# ------------------------------------------------------------------ parts ---
# Adafruit PiTFT Plus 2.8" capacitive (2423), in its own CAD frame: board
# X 0..85, Y -56..0, board top at Z 0.  [cad]
TFT_W, TFT_H, TFT_PCB = 85.0, 56.0, 1.6
TFT_HOLES = [(3.5, -52.5), (3.5, -3.5), (81.5, -52.5), (81.5, -3.5)]   # 3.0 mm
TFT_GLASS = (7.8, 77.0, -56.0, -6.1)        # display block x0, x1, y0, y1
TFT_GLASS_Z = 3.8                           # its top above the board
TFT_SWITCHES = (0.5, 6.5, -50.0, -6.8)      # four small side switches on the display side
TFT_SWITCH_Z = 5.4                          # room left above the board for them [est: CAD shows 5.0]
TFT_SOCKET = 13.4                           # socket below the board [cad]
HEADER_BASE = 2.54                          # plastic base of the Pi's header [std]

# Raspberry Pi 4 Model B: 85 x 56 board, same hole pattern as the display
# board [cad, via the two boards that bolt to it]. Connector overhangs are
# not from a drawing, so the walls stand well clear of them. [est]
PI_PCB = 1.4
PI_USB_OVERHANG = 4.0        # room left of the USB and Ethernet sockets
PI_PORT_OVERHANG = 3.5       # room above the HDMI, power and audio sockets

# PiSugar 3 Plus, under the Pi. [cad]
SUGAR_NUTS = 3.0             # Pi underside to the PiSugar board
SUGAR_PCB = 1.0
SUGAR_BATTERY = 11.1         # room under its board in PiSugar's own case [cad]
SUGAR_USBC_X = 11.5          # USB-C centre, in the Pi's X (from the SD card end)
SUGAR_USBC = (9.0, 3.2)      # USB-C shell width, height; it hangs under the PiSugar board
SUGAR_BUTTONS_X = (21.7, 30.2)   # two edge buttons, in the Pi's X

# Adafruit enclosed speaker 3351: sound side down, flange holes 3.0 mm. [cad]
SPK_W, SPK_H, SPK_T = 70.0, 30.0, 19.7
SPK_HOLES = [(3, 3), (67, 3), (67, 27), (3, 27)]
# Adafruit I2S microphone 6049. [cad]
MIC_W, MIC_H = 16.51, 12.70
# Cherry MX style switch. [std]
KEY_HOLE, KEY_PLATE = 14.0, 1.5
KEY_STEM_TOP = 10.2          # plate top to the top of the stem
LED_D = 5.1
MAGNET_D, MAGNET_T = 10.0, 1.5

# ----------------------------------------------------------------- shell ---
FIT = 0.3
WALL = 2.4
W, H = 162.0, 100.0
FOAM = 2.0                   # slack under the battery, filled with soft foam
R = 8.0
SCREW_HOLE, SCREW_PASS = 2.8, 3.4   # M3 cuts its own thread in the first

# Heights, from the back.
PI_BOTTOM = WALL + FOAM + SUGAR_BATTERY + SUGAR_PCB + SUGAR_NUTS
PI_TOP = PI_BOTTOM + PI_PCB
TFT_TOP = PI_TOP + HEADER_BASE + TFT_SOCKET + TFT_PCB       # top of the display board
FACE_UNDER = round(TFT_TOP + TFT_GLASS_Z + 0.3, 1)          # inside of the face
T = FACE_UNDER + WALL                                       # front surface
DECK_DROP = 10.5             # how far the key switches sit below the front
LID_RIM = 5.5
LID = LID_RIM + WALL

# Where the Pi stack sits: its board's lower-left corner. The stack is
# turned half a turn, so the ports face the top edge and the header the
# speaker.
SX = WALL + PI_USB_OVERHANG
SY = H - WALL - PI_PORT_OVERHANG - TFT_H


def tft(x, y):
    """A point in the display board's CAD frame, in case coordinates."""
    return (SX + TFT_W - x, SY - y)


def pi(x):
    """An X position on the Pi (from its SD card end), in case coordinates."""
    return SX + TFT_W - x


SPEAKER = (14.0, 4.5)                       # lower-left corner of the speaker
SPK_POSTS = [(SPEAKER[0] - 6.5, 19.5), (SPEAKER[0] + SPK_W + 6.5, 19.5)]
DECK = (101.0, 147.0, 6.0, 76.0)            # opening for the key deck: x0, x1, y0, y1
KEYS = {"yes": (113, 41), "no": (135, 41), "not_sure": (113, 19), "play": (135, 19), "power": (124, 63)}
DECK_EARS = [(108.0, 80.6), (140.0, 80.6)]
MIC = (116.0, 91.0)
LED = (136.0, 91.0)
MAGNETS = [(154.2, 16.0), (154.2, 84.0)]
POSTS = [(5.5, 5.5), (W - 5.5, 5.5), (W - 5.5, H - 5.5), (97.0, H - 5.5)]
HINGE_X, HINGE_Z, HINGE_R = -4.5, T - 4.5, 4.0
HINGE_BASE = [(10, 26), (42, 58), (74, 90)]     # knuckles on the body, along Y
HINGE_LID = [(26.4, 41.6), (58.4, 73.6)]        # knuckles on the lid
PIN_FREE, PIN_GRIP = 3.4, 3.0                   # bores for a 3 mm pin


# --------------------------------------------------------------- helpers ---
def rounded(w, h, z0, z1, r, x=0.0, y=0.0):
    return (cq.Workplane("XY").box(w, h, z1 - z0, centered=(False, False, False))
            .edges("|Z").fillet(r).translate((x, y, z0)))


def box(x0, x1, y0, y1, z0, z1):
    return cq.Workplane("XY").box(x1 - x0, y1 - y0, z1 - z0, centered=(False, False, False)).translate((x0, y0, z0))


def cyl(x, y, d, z0, z1):
    return cq.Workplane("XY").circle(d / 2).extrude(z1 - z0).translate((x, y, z0))


def ycyl(x, z, d, y0, y1):
    """A cylinder lying along Y."""
    return cq.Workplane("XZ").circle(d / 2).extrude(-(y1 - y0)).translate((x, y0, z))


def knuckle(y0, y1, z_far):
    """A hinge knuckle: a barrel on the hinge line with a web running up or
    down to z_far, so it prints without overhang."""
    web = box(HINGE_X - HINGE_R, 0.2, y0, y1, min(HINGE_Z, z_far), max(HINGE_Z, z_far))
    return web.union(ycyl(HINGE_X, HINGE_Z, 2 * HINGE_R, y0, y1))


# ----------------------------------------------------------------- parts ---
def front():
    body = rounded(W, H, WALL, T, R)
    body = body.faces(">Z").edges().chamfer(1.0)
    body = body.cut(rounded(W - 2 * WALL, H - 2 * WALL, WALL - 1, FACE_UNDER, R - WALL, WALL, WALL))
    inside = rounded(W, H, WALL, T, R)

    add = cq.Workplane("XY")
    # Posts for the back cover screws.
    for x, y in POSTS:
        add = add.union(cyl(x, y, 8, WALL + 2.4, FACE_UNDER + 0.1))
    # The display board rests on four pads and is located by pins in its
    # mounting holes.
    for hx, hy in TFT_HOLES:
        x, y = tft(hx, hy)
        add = add.union(cyl(x, y, 6.0, TFT_TOP, FACE_UNDER + 0.1)).union(cyl(x, y, 2.7, TFT_TOP - 1.4, TFT_TOP + 0.1))
    # Speaker: pegs in its flange holes, and two posts for the clamp.
    for hx, hy in SPK_HOLES:
        add = add.union(cyl(SPEAKER[0] + hx, SPEAKER[1] + hy, 2.6, FACE_UNDER - 2.2, FACE_UNDER + 0.1))
    for x, y in SPK_POSTS:
        add = add.union(cyl(x, y, 7, FACE_UNDER - SPK_T, FACE_UNDER + 0.1))
    # Posts the key deck screws to.
    for x, y in DECK_EARS:
        add = add.union(cyl(x, y, 6.5, T - DECK_DROP + 1.6, FACE_UNDER + 0.1))
    # Side ribs for the microphone board.
    for side in (-1, 1):
        x = MIC[0] + side * (MIC_W / 2 + FIT + 0.6)
        add = add.union(box(x - 0.6, x + 0.6, MIC[1] - 5, MIC[1] + 5, FACE_UNDER - 2.0, FACE_UNDER + 0.1))
    body = body.union(add.intersect(inside))

    # Hinge knuckles on the left wall, flat with the front so the part
    # prints face down.
    for y0, y1 in HINGE_BASE:
        body = body.union(knuckle(y0, y1, T))
    body = body.cut(ycyl(HINGE_X, HINGE_Z, PIN_GRIP, HINGE_BASE[0][0] + 2, HINGE_BASE[-1][1] + 1))

    # Screen window: the whole display block shows, less a 1.2 mm border.
    gx0, gy1 = tft(TFT_GLASS[1] - 1.2, TFT_GLASS[2] + 1.2)
    gx1, gy0 = tft(TFT_GLASS[0] + 1.2, TFT_GLASS[3] - 1.2)
    cx, cy, ww, wh = (gx0 + gx1) / 2, (gy0 + gy1) / 2, gx1 - gx0, gy1 - gy0
    window = (cq.Workplane("XY").rect(ww, wh).workplane(offset=WALL + 0.2).rect(ww + 4, wh + 4).loft()
              .translate((cx, cy, FACE_UNDER - 0.1)))
    body = body.cut(window)
    # Room for the display board's own small switches.
    sx0, sy1 = tft(TFT_SWITCHES[1], TFT_SWITCHES[2])
    sx1, sy0 = tft(TFT_SWITCHES[0], TFT_SWITCHES[3])
    body = body.cut(box(sx0, sx1, sy0, sy1, FACE_UNDER - 1, TFT_TOP + TFT_SWITCH_Z))

    # Speaker grille.
    for i in range(18):
        x = SPEAKER[0] + 6.5 + i * 3.4
        body = body.cut(box(x, x + 1.7, SPEAKER[1] + 4, SPEAKER[1] + SPK_H - 4, FACE_UNDER - 1, T + 1))
    # Key deck opening, LED, microphone, magnets, screw holes.
    body = body.cut(box(DECK[0], DECK[1], DECK[2], DECK[3], FACE_UNDER - 1, T + 1).edges("|Z").fillet(2.0))
    body = body.cut(cyl(*LED, LED_D, FACE_UNDER - 1, T + 1))
    body = body.cut(cyl(*MIC, 3.0, FACE_UNDER - 1, T + 1))
    for x, y in MAGNETS:
        body = body.cut(cyl(x, y, MAGNET_D + FIT, FACE_UNDER - 1, FACE_UNDER + MAGNET_T + 0.3))
    for x, y in POSTS:
        body = body.cut(cyl(x, y, SCREW_HOLE, WALL, WALL + 2.4 + 14))
    for x, y in SPK_POSTS + DECK_EARS:
        z = FACE_UNDER - SPK_T if (x, y) in SPK_POSTS else T - DECK_DROP + 1.6
        body = body.cut(cyl(x, y, SCREW_HOLE, z - 1, z + 8))

    # Top edge: charging port, two pin holes for the PiSugar's own buttons,
    # and vents beside the Pi.
    sugar_under = PI_BOTTOM - SUGAR_NUTS - SUGAR_PCB
    port_z = sugar_under - SUGAR_USBC[1] / 2
    body = body.cut(box(pi(SUGAR_USBC_X) - 6.5, pi(SUGAR_USBC_X) + 6.5, H - 2 * WALL, H + 1, port_z - 3.75, port_z + 3.75)
                    .edges("|Y").fillet(1.5))
    for bx in SUGAR_BUTTONS_X:
        body = body.cut(cq.Workplane("XZ").circle(1.2).extrude(-3 * WALL).translate((pi(bx), H - 2 * WALL, sugar_under - 0.8)))
    for i in range(7):
        x = 22 + i * 6.0
        body = body.cut(box(x, x + 2.5, H - 2 * WALL, H + 1, PI_TOP + 3, PI_TOP + 13))
    return body


def front_inlay():
    """A thin frame around the screen, for a second colour."""
    gx0, gy1 = tft(TFT_GLASS[1] - 1.2, TFT_GLASS[2] + 1.2)
    gx1, gy0 = tft(TFT_GLASS[0] + 1.2, TFT_GLASS[3] - 1.2)
    outer = box(gx0 - 6, gx1 + 6, gy0 - 5.5, gy1 + 5.5, T - 0.6, T).edges("|Z").fillet(3.0)
    return outer.intersect(front())


def back():
    plate = rounded(W, H, 0, WALL, R)
    plate = plate.faces("<Z").edges().chamfer(1.0)
    rim = rounded(W - 2 * WALL - 2 * FIT, H - 2 * WALL - 2 * FIT, WALL, WALL + 2.0, R - WALL, WALL + FIT, WALL + FIT)
    rim = rim.cut(rounded(W - 2 * WALL - 2 * FIT - 3.2, H - 2 * WALL - 2 * FIT - 3.2, WALL - 1, WALL + 3, R - WALL - 1.6,
                          WALL + FIT + 1.6, WALL + FIT + 1.6))
    for x, y in POSTS:
        rim = rim.cut(cyl(x, y, 10, WALL - 1, WALL + 3))
    part = plate.union(rim)
    for x, y in POSTS:
        part = part.cut(cyl(x, y, SCREW_PASS, -1, WALL + 3)).cut(cyl(x, y, 6.4, -1, 1.2))
    # Vents under the Pi's half of the case.
    for i in range(8):
        x = 30 + i * 7.0
        part = part.cut(box(x, x + 2.5, 8, 30, -1, WALL + 1))
    return part


def deck():
    """The sunken tray the five key switches snap into. Printed plate down,
    in its own colour."""
    x0, x1, y0, y1 = DECK
    floor_top = T - DECK_DROP
    t = 1.6
    tray = box(x0 - t, x1 + t, y0 - t, y1 + t, floor_top - KEY_PLATE, FACE_UNDER - 0.2).edges("|Z").fillet(2.0 + t)
    tray = tray.cut(box(x0, x1, y0, y1, floor_top, T).edges("|Z").fillet(2.0))
    for x, y in DECK_EARS:
        ear = box(x - 5, x + 5, y1 + t - 0.5, y + 3.3, floor_top - KEY_PLATE, floor_top + 1.6)
        tray = tray.union(ear).cut(cyl(x, y, SCREW_PASS, floor_top - 3, floor_top + 3))
    for x, y in KEYS.values():
        tray = tray.cut(box(x - KEY_HOLE / 2, x + KEY_HOLE / 2, y - KEY_HOLE / 2, y + KEY_HOLE / 2, floor_top - 3, floor_top + 1))
    return tray


def lid():
    plate = rounded(W, H, T + LID_RIM, T + LID, R)
    plate = plate.faces(">Z").edges().chamfer(1.0)
    rim = rounded(W, H, T + 0.2, T + LID_RIM, R).cut(rounded(W - 4, H - 4, T - 1, T + LID_RIM + 1, R - 2, 2, 2))
    part = plate.union(rim)
    # Magnets ride in bosses that reach down to the front of the body. The
    # pockets are closed: pause the print to drop the magnets in.
    for x, y in MAGNETS:
        part = part.union(cyl(x, y, MAGNET_D + 4, T + 0.2, T + LID_RIM + 0.1))
        part = part.cut(cyl(x, y, MAGNET_D + FIT, T + 0.2 + 0.6, T + 0.2 + 0.6 + MAGNET_T + 0.2))
    # A lip to open it by.
    part = part.union(box(W - 1, W + 3.5, H / 2 - 9, H / 2 + 9, T + LID_RIM, T + LID).edges("|Z").fillet(1.5))
    for y0, y1 in HINGE_LID:
        part = part.union(knuckle(y0, y1, T + LID).cut(box(-0.35, 1, y0 - 1, y1 + 1, 0, T + 0.2)))
    part = part.cut(ycyl(HINGE_X, HINGE_Z, PIN_FREE, HINGE_LID[0][0] - 1, HINGE_LID[-1][1] + 1))
    return part


def lid_inlay():
    """A band and a disc let into the lid, for a second colour."""
    z0, z1 = T + LID - 0.6, T + LID
    band = box(0, W, H * 0.30, H * 0.30 + 9, z0, z1)
    disc = cyl(W * 0.28, H * 0.30 + 4.5, 34, z0, z1).cut(cyl(W * 0.28, H * 0.30 + 4.5, 20, z0 - 1, z1 + 1))
    return band.union(disc).intersect(lid())


def clamp():
    """Bar across the back of the speaker."""
    span = SPK_POSTS[1][0] - SPK_POSTS[0][0]
    bar = rounded(span + 8, 8, 0, 3, 3.9, -4, -4)
    for x in (0, span):
        bar = bar.cut(cyl(x, 0, SCREW_PASS, -1, 4))
    return bar


def keycap(symbol, size=20.0, height=7.5):
    """A cap for an MX style switch, printed top face down."""
    cap = cq.Workplane("XY").rect(size, size).workplane(offset=height).rect(size - 3, size - 3).loft()
    cap = cap.edges("not(<Z or >Z)").fillet(1.5)
    hollow = (cq.Workplane("XY").rect(size - 3, size - 3).workplane(offset=height - 1.6).rect(size - 6, size - 6).loft()
              .translate((0, 0, -0.01)))
    cap = cap.cut(hollow)
    stem = cyl(0, 0, 5.6, 0.3, height - 1.5)
    cross = box(-2.1, 2.1, -0.7, 0.7, 0, 4.3).union(box(-0.7, 0.7, -2.1, 2.1, 0, 4.3))
    cap = cap.union(stem.cut(cross))
    return cap.cut(keymark(symbol, height))


def keymark(symbol, height=7.5):
    """The sunken symbol on a keycap; also exported on its own so it can be
    printed in another colour."""
    def bar(length, angle):
        return box(-length / 2, length / 2, -1.1, 1.1, 0, 0.6).rotate((0, 0, 0), (0, 0, 1), angle)
    if symbol == "yes":
        mark = cq.Workplane("XY").polyline([(-5, 0), (-2, -4), (5.5, 4), (4, 5), (-2, -1.2), (-3.8, 1.2)]).close().extrude(0.6)
    elif symbol == "no":
        mark = bar(12, 45).union(bar(12, -45))
    elif symbol == "not_sure":
        arc = cq.Workplane("XY").circle(3.6).circle(1.6).extrude(0.6).cut(box(-6, 0, -4, 1, -1, 2)).translate((0, 2.2, 0))
        mark = arc.union(box(-1, 1, -3.5, -0.5, 0, 0.6)).union(cyl(0, -5.2, 2.2, 0, 0.6))
    elif symbol == "play":
        mark = cq.Workplane("XY").polyline([(-3.5, -5), (5, 0), (-3.5, 5)]).close().extrude(0.6)
    else:
        ring = cq.Workplane("XY").circle(4.6).circle(3.0).extrude(0.6).cut(box(-2.2, 2.2, 1.5, 6.5, -1, 2))
        mark = ring.union(box(-0.85, 0.85, 0.5, 6, 0, 0.6))
    return mark.translate((0, 0, height - 0.6))


def fit_test():
    """The front few millimetres of the body: every opening, pad and pin,
    in a short print, to try the parts in before printing the real thing."""
    return front().intersect(box(-20, W + 20, -20, H + 20, TFT_TOP - 2.0, T + 1))


def build():
    f, l = front(), lid()
    fi, li = front_inlay(), lid_inlay()
    parts = {
        "front": f.cut(fi), "front_inlay": fi, "back": back(), "deck": deck(),
        "lid": l.cut(li), "lid_inlay": li, "speaker_clamp": clamp(), "fit_test": fit_test(),
    }
    for name in KEYS:
        size, height = (18.0, 6.0) if name == "power" else (20.0, 7.5)
        parts["keycap_" + name] = keycap(name, size, height)
        parts["keymark_" + name] = keymark(name, height)
    return parts


if __name__ == "__main__":
    out = os.path.join(os.path.dirname(os.path.abspath(__file__)), "out")
    os.makedirs(out, exist_ok=True)
    print(f"body {W:.0f} x {H:.0f} x {T:.1f} mm, {T + LID:.1f} mm with the lid closed")
    print(f"heights from the back: Pi board top {PI_TOP:.2f}, display board top {TFT_TOP:.2f}, inside of front {FACE_UNDER:.2f}")
    for name, part in build().items():
        solids = part.solids().vals()
        bb = part.val().BoundingBox()
        ok = all(s.isValid() for s in solids) and (len(solids) == 1 or name.endswith("inlay") or name.startswith("keymark"))
        print(f"  {name:18s} {bb.xlen:6.1f} x {bb.ylen:5.1f} x {bb.zlen:5.1f}  {sum(s.Volume() for s in solids) / 1000:6.1f} cm3  "
              f"{'ok' if ok else 'CHECK: ' + str(len(solids)) + ' solids'}")
        cq.exporters.export(part, os.path.join(out, name + ".stl"), tolerance=0.05, angularTolerance=0.2)
        cq.exporters.export(part, os.path.join(out, name + ".step"))
