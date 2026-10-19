#!/usr/bin/env python3
"""Fabrique lumen.ico et les fichiers rsrc_windows_*.syso (icône de l'exécutable).

Usage : python3 assets/make_syso.py   (depuis la racine du projet ; demande Pillow)
Go inclut automatiquement les fichiers .syso dans l'exécutable Windows."""
import io, struct, os
from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
SIZES = [16, 24, 32, 48, 64, 128, 256]
src = Image.open(os.path.join(HERE, "lumen-1024.png")).convert("RGBA")

def dib(img):
    """Image ICO classique : en-tête BITMAPINFOHEADER, pixels BGRA de bas en haut, masque AND."""
    w, h = img.size
    px = img.load()
    rows = []
    for y in range(h - 1, -1, -1):
        rows.append(b"".join(struct.pack("BBBB", px[x, y][2], px[x, y][1], px[x, y][0], px[x, y][3]) for x in range(w)))
    xor = b"".join(rows)
    stride = ((w + 31) // 32) * 4
    andmask = b"\x00" * (stride * h)
    hdr = struct.pack("<IiiHHIIiiII", 40, w, h * 2, 1, 32, 0, len(xor) + len(andmask), 0, 0, 0, 0)
    return hdr + xor + andmask

def png(img):
    b = io.BytesIO()
    img.save(b, "PNG", optimize=True)
    return b.getvalue()

images = []
for s in SIZES:
    im = src.resize((s, s), Image.LANCZOS)
    images.append((s, png(im) if s >= 256 else dib(im)))

# --- lumen.ico
ico = struct.pack("<HHH", 0, 1, len(images))
off = 6 + 16 * len(images)
body = b""
for s, data in images:
    ico += struct.pack("<BBBBHHII", s % 256, s % 256, 0, 0, 1, 32, len(data), off + len(body))
    body += data
open(os.path.join(HERE, "lumen.ico"), "wb").write(ico + body)
src.resize((256, 256), Image.LANCZOS).save(os.path.join(HERE, "lumen-256.png"))

# --- ressource : RT_ICON (3) x N + RT_GROUP_ICON (14) nom 1
grp = struct.pack("<HHH", 0, 1, len(images))
for i, (s, data) in enumerate(images, 1):
    grp += struct.pack("<BBBBHHIH", s % 256, s % 256, 0, 0, 1, 32, len(data), i)

leaves = {3: [(i, d) for i, (s, d) in enumerate(images, 1)], 14: [(1, grp)]}

def build_rsrc():
    """Arbre de ressources ; renvoie (octets, liste des décalages des champs OffsetToData)."""
    types = sorted(leaves)
    # taille des répertoires
    def dsize(n): return 16 + 8 * n
    root_sz = dsize(len(types))
    pos = root_sz
    type_dir = {}
    for t in types:
        type_dir[t] = pos
        pos += dsize(len(leaves[t]))
    lang_dir = {}
    for t in types:
        for (rid, _) in leaves[t]:
            lang_dir[(t, rid)] = pos
            pos += dsize(1)
    entry_pos = {}
    for t in types:
        for (rid, _) in leaves[t]:
            entry_pos[(t, rid)] = pos
            pos += 16
    data_pos = {}
    for t in types:
        for (rid, d) in leaves[t]:
            pos = (pos + 3) & ~3
            data_pos[(t, rid)] = pos
            pos += len(d)
    total = (pos + 3) & ~3
    buf = bytearray(total)
    relocs = []
    def put_dir(at, entries):
        struct.pack_into("<IIHHHH", buf, at, 0, 0, 0, 0, 0, len(entries))
        for k, (ident, target, is_dir) in enumerate(entries):
            struct.pack_into("<II", buf, at + 16 + 8 * k, ident, target | (0x80000000 if is_dir else 0))
    put_dir(0, [(t, type_dir[t], True) for t in types])
    for t in types:
        put_dir(type_dir[t], [(rid, lang_dir[(t, rid)], True) for (rid, _) in leaves[t]])
        for (rid, d) in leaves[t]:
            put_dir(lang_dir[(t, rid)], [(0x0409, entry_pos[(t, rid)], False)])
            ep = entry_pos[(t, rid)]
            struct.pack_into("<IIII", buf, ep, data_pos[(t, rid)], len(d), 0, 0)  # le premier champ est relogé (RVA)
            relocs.append(ep)
            buf[data_pos[(t, rid)]:data_pos[(t, rid)] + len(d)] = d
    return bytes(buf), relocs

rsrc, relocs = build_rsrc()

ARCH = {  # nom Go -> (Machine, type de relocation « adresse 32 bits relative à la base »)
    "amd64": (0x8664, 3),   # IMAGE_REL_AMD64_ADDR32NB
    "386":   (0x014C, 7),   # IMAGE_REL_I386_DIR32NB
    "arm64": (0xAA64, 2),   # IMAGE_REL_ARM64_ADDR32NB
}
for goarch, (machine, rtype) in ARCH.items():
    raw_off = 20 + 40
    reloc_off = raw_off + len(rsrc)
    sym_off = reloc_off + 10 * len(relocs)
    hdr = struct.pack("<HHIIIHH", machine, 1, 0, sym_off, 1, 0, 0)  # 1 symbole (+1 auxiliaire = 2 entrées)
    hdr = struct.pack("<HHIIIHH", machine, 1, 0, sym_off, 2, 0, 0)
    sec = struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(rsrc), raw_off, reloc_off, 0, len(relocs), 0, 0x40000040)
    rel = b"".join(struct.pack("<IIH", r, 0, rtype) for r in relocs)
    sym = struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 1) + b"\x00" * 18
    strtab = struct.pack("<I", 4)
    name = os.path.join(ROOT, "rsrc_windows_%s.syso" % goarch)
    open(name, "wb").write(hdr + sec + rsrc + rel + sym + strtab)
    print("écrit", os.path.relpath(name, ROOT))
print("écrit assets/lumen.ico", len(ico + body), "octets")
