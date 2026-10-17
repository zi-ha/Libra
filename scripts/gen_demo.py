"""生成一批带真实封面的示例电子书，用于本地跑起来直观查看效果。

用法：python scripts/gen_demo.py
输出到 ./testdata/demo/，其中 tech/ 与 literature/ 可作为两个独立的媒体库目录添加。

封面用纯 Python（zlib + struct）手写 PNG 编码器生成：竖向渐变 + 几何图形，
不依赖任何第三方图像库。
"""
import math
import os
import struct
import zipfile
import zlib

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BASE = os.path.join(ROOT, "testdata", "demo")

W, H = 420, 630


# --------------------------------------------------------------------- PNG 编码

def _chunk(tag, data):
    return (struct.pack(">I", len(data)) + tag + data
            + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF))


def write_png(path, rows):
    raw = b"".join(b"\x00" + r for r in rows)
    blob = b"\x89PNG\r\n\x1a\n"
    blob += _chunk(b"IHDR", struct.pack(">IIBBBBB", W, H, 8, 2, 0, 0, 0))
    blob += _chunk(b"IDAT", zlib.compress(raw, 9))
    blob += _chunk(b"IEND", b"")
    with open(path, "wb") as f:
        f.write(blob)


def blend(px, i, rgb, a):
    """把 rgb 以 alpha a 混合进字节数组 px 的第 i 个像素。"""
    if a <= 0.0:
        return
    if a > 1.0:
        a = 1.0
    px[i] = min(255, max(0, int(px[i] * (1 - a) + rgb[0] * a)))
    px[i + 1] = min(255, max(0, int(px[i + 1] * (1 - a) + rgb[1] * a)))
    px[i + 2] = min(255, max(0, int(px[i + 2] * (1 - a) + rgb[2] * a)))


def lerp(c1, c2, t):
    return (int(c1[0] + (c2[0] - c1[0]) * t),
            int(c1[1] + (c2[1] - c1[1]) * t),
            int(c1[2] + (c2[2] - c1[2]) * t))


# 每个库一套配色：(渐变的暗色, 亮色, 装饰色)
PALETTES = [
    ((13, 27, 62), (32, 92, 148), (245, 176, 65)),
    ((28, 18, 44), (92, 46, 122), (238, 137, 172)),
    ((10, 38, 36), (22, 104, 96), (250, 214, 120)),
    ((48, 20, 20), (158, 58, 58), (247, 220, 186)),
    ((16, 34, 54), (56, 128, 168), (222, 240, 255)),
    ((34, 34, 34), (88, 88, 96), (200, 208, 216)),
    ((44, 32, 12), (176, 124, 42), (255, 236, 198)),
    ((18, 24, 40), (68, 78, 132), (168, 190, 255)),
]


def make_cover(path, style, pal):
    c1, c2, accent = pal
    rows = []
    cx, cy, r = W * 0.72, H * 0.30, W * 0.38

    for y in range(H):
        t = y / (H - 1)
        base = lerp(c1, c2, t * t)  # 平方使上暗下亮过渡更自然
        px = bytearray(base * W)

        if style == 0:
            # 半透明圆
            dy = y - cy
            if abs(dy) <= r:
                half = math.sqrt(r * r - dy * dy)
                x0, x1 = int(cx - half), int(cx + half)
                for x in range(max(0, x0), min(W, x1)):
                    d = math.hypot(x - cx, dy) / r
                    blend(px, x * 3, accent, 0.30 * (1 - d * 0.55))
        elif style == 1:
            # 斜切三角
            edge = int(W * 1.15 - y * 0.72)
            for x in range(0, min(W, max(0, edge))):
                blend(px, x * 3, accent, 0.22)
        elif style == 2:
            # 同心弧（只在实际覆盖的圆域内计算，避免整行空转）
            ox, oy, R = W * 0.30, H * 0.78, W * 0.95
            dy2 = y - oy
            if abs(dy2) <= R:
                half = math.sqrt(R * R - dy2 * dy2)
                x0, x1 = max(0, int(ox - half)), min(W, int(ox + half) + 1)
                for x in range(x0, x1):
                    dd = math.hypot(x - ox, dy2)
                    if dd % 46.0 < 9.0:
                        blend(px, x * 3, accent, 0.26 * (1 - dd / R))
        else:
            # 底部横条组
            band = y - int(H * 0.62)
            if band >= 0:
                k = (band // 26) % 4
                if k < 2:
                    for x in range(int(W * 0.10), int(W * 0.10) + int(W * (0.80 - k * 0.22))):
                        blend(px, x * 3, accent, 0.32)

        # 左侧书脊压条
        if y > H * 0.06:
            for x in range(int(W * 0.055), int(W * 0.075)):
                blend(px, x * 3, (255, 255, 255), 0.10)

        # 底部署名区暗化
        if y > H * 0.86:
            for x in range(W):
                blend(px, x * 3, (0, 0, 0), 0.22)

        rows.append(bytes(px))

    write_png(path, rows)


# --------------------------------------------------------------------- EPUB

CONTAINER = """<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
"""

OPF = """<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="{ver}" unique-identifier="BookId">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>{title}</dc:title>
    <dc:creator>{author}</dc:creator>
{extra}
  </metadata>
  <manifest>
    <item id="cover-img" href="images/cover.png" media-type="image/png"{props}/>
  </manifest>
  <spine/>
</package>
"""


def make_epub(path, title, author, cover_png, version="2.0"):
    extra = ('    <meta name="cover" content="cover-img"/>' if version == "2.0"
             else '    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>')
    props = '' if version == "2.0" else ' properties="cover-image"'
    opf = OPF.format(ver=version, title=title, author=author, extra=extra, props=props)

    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as z:
        zi = zipfile.ZipInfo("mimetype")
        zi.compress_type = zipfile.ZIP_STORED
        z.writestr(zi, "application/epub+zip")
        z.writestr("META-INF/container.xml", CONTAINER)
        z.writestr("OEBPS/content.opf", opf)
        z.writestr("OEBPS/images/cover.png", cover_png)
        z.writestr("OEBPS/chapter1.xhtml",
                   '<?xml version="1.0" encoding="UTF-8"?>'
                   '<html xmlns="http://www.w3.org/1999/xhtml"><head>'
                   '<title>%s</title></head><body><h1>%s</h1></body></html>'
                   % (title, title))
    print("epub ->", os.path.relpath(path, ROOT))


# --------------------------------------------------------------------- PDF

def make_pdf(path, title):
    """手写结构合法的最小单页 PDF（正确的 xref 偏移）。"""
    objs = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 420 630] "
        b"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
    ]
    stream = b"BT /F1 20 Tf 48 560 Td (Libra PDF Sample) Tj ET"
    objs.append(b"<< /Length " + str(len(stream)).encode() + b" >>\nstream\n" + stream + b"\nendstream")
    objs.append(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

    out = bytearray(b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
    offsets = []
    for i, body in enumerate(objs, start=1):
        offsets.append(len(out))
        out += str(i).encode() + b" 0 obj\n" + body + b"\nendobj\n"

    xref = len(out)
    n = len(objs) + 1
    out += b"xref\n0 " + str(n).encode() + b"\n0000000000 65535 f \n"
    for off in offsets:
        out += ("%010d 00000 n \n" % off).encode()
    out += (b"trailer\n<< /Size " + str(n).encode() + b" /Root 1 0 R >>\n"
            b"startxref\n" + str(xref).encode() + b"\n%%EOF\n")

    with open(path, "wb") as f:
        f.write(out)
    print("pdf  ->", os.path.relpath(path, ROOT))


# --------------------------------------------------------------------- 数据

TECH = [
    ("深入理解计算机系统", "Randal E. Bryant", "2.0"),
    ("Designing Data-Intensive Applications", "Martin Kleppmann", "3.0"),
    ("The Go Programming Language", "Alan A. A. Donovan", "3.0"),
    ("重构：改善既有代码的设计", "Martin Fowler", "2.0"),
    ("计算机网络：自顶向下方法", "James F. Kurose", "2.0"),
    ("Compilers: Principles and Techniques", "Alfred V. Aho", "3.0"),
]

LIT = [
    ("Sapiens: A Brief History of Humankind", "Yuval Noah Harari", "3.0"),
    ("百年孤独", "Gabriel García Márquez", "2.0"),
    ("The Remains of the Day", "Kazuo Ishiguro", "3.0"),
    ("活着", "余华", "2.0"),
    ("Norwegian Wood", "Haruki Murakami", "3.0"),
    ("The Old Man and the Sea", "Ernest Hemingway", "2.0"),
]


def safe_name(title):
    """标题 -> 合法文件名。Windows 不允许文件名含 : / \\ * ? " < > |。"""
    out = title
    for ch in ':/\\*?"<>|':
        out = out.replace(ch, "：")
    return out.strip(" .")


def main():
    os.makedirs(os.path.join(BASE, "tech", "编译原理"), exist_ok=True)
    os.makedirs(os.path.join(BASE, "literature"), exist_ok=True)

    tmp = os.path.join(BASE, ".cover.png")

    def cover(i, style):
        make_cover(tmp, style, PALETTES[i % len(PALETTES)])
        with open(tmp, "rb") as f:
            return f.read()

    for i, (title, author, ver) in enumerate(TECH):
        png = cover(i, i % 4)
        sub = "编译原理" if i == 5 else ""
        make_epub(os.path.join(BASE, "tech", sub, safe_name(title) + ".epub"),
                  title, author, png, ver)

    for i, (title, author, ver) in enumerate(LIT):
        png = cover(i + 3, (i + 1) % 4)
        make_epub(os.path.join(BASE, "literature", safe_name(title) + ".epub"),
                  title, author, png, ver)

    # 两个 PDF：本机没有 poppler 时封面会走降级，容器里才会渲染
    make_pdf(os.path.join(BASE, "tech", "编译原理", "Dragon Book 影印版.pdf"), "Compilers")
    make_pdf(os.path.join(BASE, "literature", "散文集选读.pdf"), "Essays")

    if os.path.exists(tmp):
        os.remove(tmp)
    print("\n共 14 本，输出目录：", os.path.relpath(BASE, ROOT))


if __name__ == "__main__":
    main()
