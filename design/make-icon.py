import math, sys

def squircle(cx, cy, half, n=5.0, steps=720):
    # Superellipse close to the continuous-corner shape of macOS icons.
    pts = []
    for i in range(steps):
        t = 2 * math.pi * i / steps
        c, s = math.cos(t), math.sin(t)
        x = half * math.copysign(abs(c) ** (2 / n), c)
        y = half * math.copysign(abs(s) ** (2 / n), s)
        pts.append(f"{cx + x:.2f},{cy + y:.2f}")
    return "M" + " L".join(pts) + " Z"

def continuous_rect(x, y, size, r):
    # Rounded square whose corners ease into the sides (no visible kink),
    # like the macOS icon shape: each corner is a superellipse quadrant
    # spread over 1.5r of the side.
    e = r * 1.52
    pts = []
    corners = [(x + size - e, y + e, 1, -1), (x + size - e, y + size - e, 1, 1),
               (x + e, y + size - e, -1, 1), (x + e, y + e, -1, -1)]
    starts = [-math.pi / 2, 0, math.pi / 2, math.pi]
    for (cx, cy, _, _), a0 in zip(corners, starts):
        for i in range(91):
            t = a0 + (math.pi / 2) * i / 90
            c, s_ = math.cos(t), math.sin(t)
            px = cx + e * math.copysign(abs(c) ** (2 / 4.6), c)
            py = cy + e * math.copysign(abs(s_) ** (2 / 4.6), s_)
            pts.append(f"{px:.2f},{py:.2f}")
    return "M" + " L".join(pts) + " Z"

body = continuous_rect(100, 100, 824, 185)

# A card: rounded photo with a white mat, rotated about its centre.
def card(cx, cy, w, h, rot, photo_id, mat=16, extra=""):
    x, y = cx - w / 2, cy - h / 2
    return f'''
  <g transform="rotate({rot} {cx} {cy})" filter="url(#cardShadow)">
    <rect x="{x}" y="{y}" width="{w}" height="{h}" rx="34" fill="#fbfaf7"/>
    <g clip-path="url(#clip-{photo_id})">
      <use href="#photo-{photo_id}"/>
    </g>
    {extra}
  </g>'''

def clip(pid, cx, cy, w, h, mat):
    return f'<clipPath id="clip-{pid}"><rect x="{cx - w/2 + mat}" y="{cy - h/2 + mat}" width="{w - 2*mat}" height="{h - 2*mat}" rx="22"/></clipPath>'

# Card geometry (centre, size, rotation)
L = dict(cx=372, cy=470, w=330, h=410, rot=-14)
R = dict(cx=652, cy=470, w=330, h=410, rot=13)
F = dict(cx=512, cy=520, w=420, h=460, rot=0)
MAT = 18

def box(c):
    return c["cx"] - c["w"]/2 + MAT, c["cy"] - c["h"]/2 + MAT, c["w"] - 2*MAT, c["h"] - 2*MAT

lx, ly, lw, lh = box(L)
rx_, ry, rw, rh = box(R)
fx, fy, fw, fh = box(F)
# Front photo leaves room for the palette strip under it.
strip_h = 54
fph = fh - strip_h - 14

svg = f'''<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">
<defs>
  <linearGradient id="bg" x1="0" y1="0" x2="0.35" y2="1">
    <stop offset="0" stop-color="#2b3f7a"/>
    <stop offset="0.55" stop-color="#1b2550"/>
    <stop offset="1" stop-color="#121733"/>
  </linearGradient>
  <radialGradient id="glow" cx="0.5" cy="0.38" r="0.6">
    <stop offset="0" stop-color="#6d8cff" stop-opacity="0.45"/>
    <stop offset="1" stop-color="#6d8cff" stop-opacity="0"/>
  </radialGradient>
  <linearGradient id="rim" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#ffffff" stop-opacity="0.35"/>
    <stop offset="0.5" stop-color="#ffffff" stop-opacity="0.04"/>
    <stop offset="1" stop-color="#ffffff" stop-opacity="0.12"/>
  </linearGradient>
  <filter id="iconShadow" x="-20%" y="-20%" width="140%" height="140%">
    <feGaussianBlur in="SourceAlpha" stdDeviation="14"/>
    <feOffset dy="12" result="b"/>
    <feComponentTransfer><feFuncA type="linear" slope="0.35"/></feComponentTransfer>
    <feMerge><feMergeNode/><feMergeNode in="SourceGraphic"/></feMerge>
  </filter>
  <filter id="cardShadow" x="-30%" y="-30%" width="160%" height="160%">
    <feGaussianBlur in="SourceAlpha" stdDeviation="16"/>
    <feOffset dy="18"/>
    <feComponentTransfer><feFuncA type="linear" slope="0.45"/></feComponentTransfer>
    <feMerge><feMergeNode/><feMergeNode in="SourceGraphic"/></feMerge>
  </filter>
  <clipPath id="body"><path d="{body}"/></clipPath>

  <!-- Left: warm dusk over dunes -->
  <linearGradient id="skyL" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#ff9a62"/><stop offset="0.6" stop-color="#f6c27a"/><stop offset="1" stop-color="#f3d9a4"/>
  </linearGradient>
  <g id="photo-L">
    <rect x="{lx}" y="{ly}" width="{lw}" height="{lh}" fill="url(#skyL)"/>
    <circle cx="{lx + lw*0.68}" cy="{ly + lh*0.42}" r="{lw*0.13}" fill="#fff4dc"/>
    <path d="M{lx-10},{ly+lh*0.72} C{lx+lw*0.3},{ly+lh*0.6} {lx+lw*0.6},{ly+lh*0.8} {lx+lw+10},{ly+lh*0.66} L{lx+lw+10},{ly+lh+10} L{lx-10},{ly+lh+10} Z" fill="#d9884f"/>
    <path d="M{lx-10},{ly+lh*0.86} C{lx+lw*0.35},{ly+lh*0.76} {lx+lw*0.7},{ly+lh*0.92} {lx+lw+10},{ly+lh*0.82} L{lx+lw+10},{ly+lh+10} L{lx-10},{ly+lh+10} Z" fill="#b5653a"/>
  </g>

  <!-- Right: teal sea and a pale moon -->
  <linearGradient id="skyR" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#0f5a63"/><stop offset="1" stop-color="#1a8b8a"/>
  </linearGradient>
  <g id="photo-R">
    <rect x="{rx_}" y="{ry}" width="{rw}" height="{rh}" fill="url(#skyR)"/>
    <circle cx="{rx_ + rw*0.35}" cy="{ry + rh*0.3}" r="{rw*0.1}" fill="#e7f3ee"/>
    <rect x="{rx_}" y="{ry + rh*0.64}" width="{rw}" height="{rh*0.4}" fill="#0b3f4a"/>
  </g>

  <!-- Front: blue sky, sun, golden field -->
  <linearGradient id="skyF" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#2f6fe0"/><stop offset="1" stop-color="#7fb4f5"/>
  </linearGradient>
  <linearGradient id="field" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#e7c25a"/><stop offset="1" stop-color="#c99a3a"/>
  </linearGradient>
  <g id="photo-F">
    <rect x="{fx}" y="{fy}" width="{fw}" height="{fph}" fill="url(#skyF)"/>
    <circle cx="{fx + fw*0.7}" cy="{fy + fph*0.34}" r="{fw*0.12}" fill="#fff6df"/>
    <path d="M{fx-10},{fy+fph*0.66} C{fx+fw*0.35},{fy+fph*0.56} {fx+fw*0.65},{fy+fph*0.7} {fx+fw+10},{fy+fph*0.6} L{fx+fw+10},{fy+fph+10} L{fx-10},{fy+fph+10} Z" fill="url(#field)"/>
    <path d="M{fx-10},{fy+fph*0.84} C{fx+fw*0.4},{fy+fph*0.76} {fx+fw*0.7},{fy+fph*0.9} {fx+fw+10},{fy+fph*0.82} L{fx+fw+10},{fy+fph+10} L{fx-10},{fy+fph+10} Z" fill="#a9792c"/>
  </g>
  {clip("L", L["cx"], L["cy"], L["w"], L["h"], MAT)}
  {clip("R", R["cx"], R["cy"], R["w"], R["h"], MAT)}
  <clipPath id="clip-F"><rect x="{fx}" y="{fy}" width="{fw}" height="{fph}" rx="22"/></clipPath>
  <clipPath id="clip-strip"><rect x="{fx}" y="{fy + fph + 14}" width="{fw}" height="{strip_h}" rx="16"/></clipPath>
</defs>

<g filter="url(#iconShadow)">
  <path d="{body}" fill="url(#bg)"/>
</g>
<g clip-path="url(#body)">
  <rect width="1024" height="1024" fill="url(#glow)"/>
  {card(**{k: L[k] for k in ("cx","cy","w","h")}, rot=L["rot"], photo_id="L")}
  {card(**{k: R[k] for k in ("cx","cy","w","h")}, rot=R["rot"], photo_id="R")}
  {card(**{k: F[k] for k in ("cx","cy","w","h")}, rot=F["rot"], photo_id="F",
        extra="".join(
          f'<rect x="{fx + i*fw/5}" y="{fy + fph + 14}" width="{fw/5 + 1}" height="{strip_h}" fill="{c}" clip-path="url(#clip-strip)"/>'
          for i, c in enumerate(["#2f6fe0", "#7fb4f5", "#fff1d2", "#e7c25a", "#a9792c"])))}
</g>
<path d="{body}" fill="none" stroke="url(#rim)" stroke-width="3"/>
</svg>
'''
open(sys.argv[1], "w").write(svg)
