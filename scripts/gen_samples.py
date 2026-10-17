"""生成联调用的样本文件：2 个 EPUB + 2 个 PDF。

用法：python scripts/gen_samples.py
输出到 ./testdata/books/，可直接在 Libra 里作为媒体库目录添加。
"""
import os
import zipfile

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BASE = os.path.join(ROOT, "testdata", "books")

# 1x1 透明 PNG
PNG = bytes.fromhex(
    "89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c4"
    "890000000a49444154789c63000100000500010d0a2db40000000049454e44ae426082"
)

CONTAINER = """<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
"""

OPF_TPL = """<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="{ver}" unique-identifier="BookId">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>{title}</dc:title>
    <dc:creator>{author}</dc:creator>
{extra}
  </metadata>
  <manifest>
    <item id="cover-img" href="images/cover.png" media-type="image/png"/>
  </manifest>
  <spine/>
</package>
"""


def make_epub(path, title, author, version="2.0"):
    if version == "2.0":
        extra = '    <meta name="cover" content="cover-img"/>'
    else:
        extra = '    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>'
    opf = OPF_TPL.format(ver=version, title=title, author=author, extra=extra)
    if version == "3.0":
        opf = opf.replace('<item id="cover-img" href="images/cover.png" media-type="image/png"/>',
                          '<item id="cover-img" href="images/cover.png" media-type="image/png" properties="cover-image"/>')

    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as z:
        # mimetype 必须第一个写入且不压缩
        zi = zipfile.ZipInfo("mimetype")
        zi.compress_type = zipfile.ZIP_STORED
        z.writestr(zi, "application/epub+zip")
        z.writestr("META-INF/container.xml", CONTAINER)
        z.writestr("OEBPS/content.opf", opf)
        z.writestr("OEBPS/images/cover.png", PNG)
    print("epub ->", path)


def make_pdf(path, title):
    """手写一个结构合法的最小单页 PDF（带正确的 xref 偏移）。"""
    objs = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 450] "
        b"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
    ]
    stream = b"BT /F1 18 Tf 40 380 Td (Libra PDF Sample) Tj ET"
    objs.append(b"<< /Length " + str(len(stream)).encode() + b" >>\nstream\n" + stream + b"\nendstream")
    objs.append(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

    out = bytearray(b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
    offsets = []
    for i, body in enumerate(objs, start=1):
        offsets.append(len(out))
        out += str(i).encode() + b" 0 obj\n" + body + b"\nendobj\n"

    xref_pos = len(out)
    n = len(objs) + 1
    out += b"xref\n0 " + str(n).encode() + b"\n"
    out += b"0000000000 65535 f \n"
    for off in offsets:
        out += ("%010d 00000 n \n" % off).encode()
    out += (b"trailer\n<< /Size " + str(n).encode() + b" /Root 1 0 R /Info 6 0 R >>\n"
            b"startxref\n" + str(xref_pos).encode() + b"\n%%EOF\n")

    # Info 字典以对象 6 的形式追加（简化：直接并入 Root 引用可省略，这里保持最小合法）
    out = out.replace(b"/Info 6 0 R", b"/Info 1 0 R")

    with open(path, "wb") as f:
        f.write(out)
    print("pdf  ->", path)


if __name__ == "__main__":
    os.makedirs(BASE, exist_ok=True)
    os.makedirs(os.path.join(BASE, "子目录"), exist_ok=True)

    make_epub(os.path.join(BASE, "深入理解计算机系统.epub"), "深入理解计算机系统", "Randal E. Bryant", "2.0")
    make_epub(os.path.join(BASE, "子目录", "Sapiens.epub"), "Sapiens: A Brief History of Humankind", "Yuval Noah Harari", "3.0")
    make_pdf(os.path.join(BASE, "sample-paper.pdf"), "Sample Paper")
    make_pdf(os.path.join(BASE, "子目录", "另一个文档.pdf"), "Another Doc")
