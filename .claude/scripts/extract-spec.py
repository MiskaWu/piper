#!/usr/bin/env python3
"""extract-spec.py — 從規格來源檔提取文字 + 圖片，產出結構化 extract.md

用法:
  python3 extract-spec.py <來源檔> <輸出目錄>

範例:
  python3 extract-spec.py /tmp/piper-clipboard/abc.xlsx vault/epics/epic-12345/specs

動作:
  1. 複製來源到 <輸出目錄>/sources/
  2. 提取嵌入圖片到 <輸出目錄>/sources/images/
  3. 產出 <輸出目錄>/extracts/<filename>.extract.md

支援格式: .xlsx .pptx .docx .xmind
不支援的格式會印出錯誤訊息並以 exit 1 結束。

Exit code:
  0 = 成功
  1 = 錯誤（格式不支援、檔案不存在、套件缺失）
"""

import sys
import os
import shutil
import datetime
from pathlib import Path


def ensure_package(package_name, import_name=None):
    """嘗試 import，失敗則自動安裝。"""
    import_name = import_name or package_name
    try:
        __import__(import_name)
    except ImportError:
        import subprocess
        print(f"Installing {package_name}...", file=sys.stderr)
        subprocess.check_call(
            [sys.executable, "-m", "pip", "install", "--quiet", package_name],
            stdout=subprocess.DEVNULL,
        )


def extract_xlsx(src_path, images_dir):
    """提取 xlsx: 逐 sheet、逐 row，含圖片。"""
    ensure_package("openpyxl")
    import openpyxl
    from openpyxl.drawing.image import Image as XlImage

    wb = openpyxl.load_workbook(src_path, data_only=True)
    lines = []

    for sheet_name in wb.sheetnames:
        ws = wb[sheet_name]

        # 檢查 sheet 是否全空
        has_content = False
        for row in ws.iter_rows(values_only=True):
            if any(cell is not None and str(cell).strip() for cell in row):
                has_content = True
                break
        if not has_content:
            continue

        lines.append(f"\n## Sheet: {sheet_name}")

        # 收集圖片位置 (anchor row → image)
        image_map = {}
        for idx, img in enumerate(ws._images):
            anchor_row = None
            if hasattr(img.anchor, "_from") and img.anchor._from is not None:
                anchor_row = img.anchor._from.row + 1
            elif hasattr(img.anchor, "row"):
                anchor_row = img.anchor.row + 1

            # 存圖片
            safe_sheet = sheet_name.replace("/", "-").replace(" ", "_")
            img_filename = f"{safe_sheet}-img{idx + 1}.png"
            img_path = images_dir / img_filename
            try:
                with open(img_path, "wb") as f:
                    f.write(img._data())
                if anchor_row is not None:
                    image_map.setdefault(anchor_row, []).append(img_filename)
                else:
                    # 無法定位的圖片放在 sheet 開頭
                    lines.append(f"[image: images/{img_filename}]")
            except Exception as e:
                print(f"Warning: failed to extract image in {sheet_name}: {e}", file=sys.stderr)

        # 逐 row 輸出
        for row_idx, row in enumerate(ws.iter_rows(values_only=True), start=1):
            cells = [str(cell).strip() if cell is not None else "" for cell in row]
            # 跳過全空 row
            if not any(cells):
                continue

            # 判斷是表格行（多欄有值）還是單值行
            non_empty = [c for c in cells if c]
            if len(non_empty) > 1:
                lines.append(f"<!-- Row {row_idx} --> " + " | ".join(cells))
            else:
                lines.append(f"<!-- Row {row_idx} --> " + " | ".join(non_empty))

            # 插入此 row 關聯的圖片
            if row_idx in image_map:
                for img_fn in image_map[row_idx]:
                    lines.append(f"[image: images/{img_fn}]")

    wb.close()
    return lines


def extract_pptx(src_path, images_dir):
    """提取 pptx: 逐 slide、逐 shape，含圖片。"""
    ensure_package("python-pptx", "pptx")
    from pptx import Presentation
    from pptx.enum.shapes import MSO_SHAPE_TYPE

    prs = Presentation(str(src_path))
    lines = []

    for slide_idx, slide in enumerate(prs.slides, start=1):
        title = ""
        if slide.shapes.title and slide.shapes.title.has_text_frame:
            title = slide.shapes.title.text_frame.text.strip()

        header = f"Slide {slide_idx}"
        if title:
            header += f": {title}"
        lines.append(f"\n## {header}")

        shape_idx = 0
        for shape in slide.shapes:
            # 圖片
            if shape.shape_type == MSO_SHAPE_TYPE.PICTURE:
                shape_idx += 1
                img_filename = f"slide{slide_idx}-img{shape_idx}.png"
                img_path = images_dir / img_filename
                try:
                    with open(img_path, "wb") as f:
                        f.write(shape.image.blob)
                    lines.append(f"[image: images/{img_filename}]")
                except Exception as e:
                    print(f"Warning: failed to extract image on slide {slide_idx}: {e}", file=sys.stderr)
                continue

            # 文字
            if shape.has_text_frame:
                for para in shape.text_frame.paragraphs:
                    text = para.text.strip()
                    if text and text != title:
                        indent = "  " * para.level if para.level else ""
                        lines.append(f"{indent}- {text}")

            # 表格
            if shape.has_table:
                table = shape.table
                for row_idx, row in enumerate(table.rows):
                    cells = [cell.text.strip() for cell in row.cells]
                    if any(cells):
                        lines.append(" | ".join(cells))

        # slide notes
        if slide.has_notes_slide and slide.notes_slide.notes_text_frame:
            notes = slide.notes_slide.notes_text_frame.text.strip()
            if notes:
                lines.append(f"<!-- Notes: {notes} -->")

    return lines


def extract_docx(src_path, images_dir):
    """提取 docx: 逐 paragraph + table，含圖片。"""
    ensure_package("python-docx", "docx")
    from docx import Document
    from docx.opc.constants import RELATIONSHIP_TYPE as RT

    doc = Document(str(src_path))
    lines = []

    # 提取所有嵌入圖片
    img_count = 0
    image_rids = {}
    for rel in doc.part.rels.values():
        if "image" in rel.reltype:
            img_count += 1
            img_filename = f"doc-img{img_count}{Path(rel.target_ref).suffix}"
            img_path = images_dir / img_filename
            try:
                with open(img_path, "wb") as f:
                    f.write(rel.target_part.blob)
                image_rids[rel.rId] = img_filename
            except Exception as e:
                print(f"Warning: failed to extract image: {e}", file=sys.stderr)

    for para in doc.paragraphs:
        text = para.text.strip()

        # 檢查段落中的圖片引用
        para_images = []
        for run in para.runs:
            drawing_elements = run._element.findall(
                ".//{http://schemas.openxmlformats.org/wordprocessingml/2006/main}drawing"
            ) + run._element.findall(
                ".//{http://schemas.openxmlformats.org/wordprocessingml/2006/main}pict"
            )
            if drawing_elements:
                # 嘗試找到對應的 image relationship
                blips = run._element.findall(
                    ".//{http://schemas.openxmlformats.org/drawingml/2006/main}blip"
                )
                for blip in blips:
                    embed = blip.get(
                        "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}embed"
                    )
                    if embed and embed in image_rids:
                        para_images.append(image_rids[embed])

        if not text and not para_images:
            continue

        # Heading → markdown heading
        if para.style and para.style.name.startswith("Heading"):
            try:
                level = int(para.style.name.split()[-1])
            except ValueError:
                level = 2
            level = min(level + 1, 6)  # offset by 1 since # is reserved for doc title
            lines.append(f"\n{'#' * level} {text}")
        elif text:
            lines.append(text)

        for img_fn in para_images:
            lines.append(f"[image: images/{img_fn}]")

    # Tables
    for table_idx, table in enumerate(doc.tables):
        lines.append(f"\n<!-- Table {table_idx + 1} -->")
        for row in table.rows:
            cells = [cell.text.strip() for cell in row.cells]
            if any(cells):
                lines.append(" | ".join(cells))

    return lines


def extract_xmind(src_path, images_dir):
    """提取 xmind: 解壓 JSON，遞迴走訪 topic tree。"""
    import zipfile
    import json

    lines = []
    img_count = 0

    with zipfile.ZipFile(str(src_path), "r") as zf:
        # xmind 8+ 格式: content.json
        # xmind zen 格式: content.json
        content_file = None
        for name in ["content.json", "content.xml"]:
            if name in zf.namelist():
                content_file = name
                break

        if content_file is None:
            raise ValueError("xmind 檔案中找不到 content.json 或 content.xml")

        if content_file == "content.xml":
            raise ValueError("xmind 8 (XML) 格式暫不支援，請轉存為 xmind zen 格式")

        data = json.loads(zf.read(content_file))

        # 提取附件圖片
        for name in zf.namelist():
            if name.startswith("resources/") or name.startswith("attachments/"):
                ext = Path(name).suffix.lower()
                if ext in (".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg"):
                    img_count += 1
                    img_filename = f"xmind-img{img_count}{ext}"
                    img_path = images_dir / img_filename
                    try:
                        with open(img_path, "wb") as f:
                            f.write(zf.read(name))
                    except Exception as e:
                        print(f"Warning: failed to extract xmind image {name}: {e}", file=sys.stderr)

        # 走訪 topic tree
        def walk_topic(topic, depth=0):
            title = topic.get("title", "").strip()
            if not title:
                return

            indent = "  " * depth
            if depth == 0:
                lines.append(f"\n## Topic: {title}")
            else:
                lines.append(f"{indent}- {title}")

            # 備註
            notes = topic.get("notes", {})
            if isinstance(notes, dict):
                plain = notes.get("plain", {})
                content = plain.get("content", "") if isinstance(plain, dict) else ""
                if content.strip():
                    for note_line in content.strip().split("\n"):
                        lines.append(f"{indent}  > {note_line}")

            # 子節點
            children = topic.get("children", {})
            if isinstance(children, dict):
                attached = children.get("attached", [])
                for child in attached:
                    walk_topic(child, depth + 1)
            elif isinstance(children, list):
                for child in children:
                    walk_topic(child, depth + 1)

        # data 可能是 list（多個 sheet）或 dict
        sheets = data if isinstance(data, list) else [data]
        for sheet in sheets:
            root_topic = sheet.get("rootTopic", sheet)
            walk_topic(root_topic)

    return lines


EXTRACTORS = {
    ".xlsx": extract_xlsx,
    ".pptx": extract_pptx,
    ".docx": extract_docx,
    ".xmind": extract_xmind,
}


def main():
    if len(sys.argv) < 3:
        print("用法: python3 extract-spec.py <來源檔> <輸出目錄>", file=sys.stderr)
        sys.exit(1)

    src_path = Path(sys.argv[1]).resolve()
    out_dir = Path(sys.argv[2]).resolve()

    if not src_path.exists():
        print(f"Error: 檔案不存在: {src_path}", file=sys.stderr)
        sys.exit(1)

    ext = src_path.suffix.lower()
    if ext not in EXTRACTORS:
        supported = ", ".join(EXTRACTORS.keys())
        print(f"Error: 不支援的格式 {ext}（支援: {supported}）", file=sys.stderr)
        sys.exit(1)

    # 建立目錄結構
    sources_dir = out_dir / "sources"
    images_dir = sources_dir / "images"
    extracts_dir = out_dir / "extracts"
    for d in [sources_dir, images_dir, extracts_dir]:
        d.mkdir(parents=True, exist_ok=True)

    # Step 1: 複製來源
    dest_source = sources_dir / src_path.name
    shutil.copy2(str(src_path), str(dest_source))
    print(f"Copied: {src_path.name} → sources/", file=sys.stderr)

    # Step 2: 提取
    extractor = EXTRACTORS[ext]
    content_lines = extractor(src_path, images_dir)

    # Step 3: 產出 extract.md
    today = datetime.date.today().isoformat()
    stem = src_path.stem
    extract_path = extracts_dir / f"{stem}.extract.md"

    header = [
        f"# Extract: {src_path.name}",
        f"> source: `{src_path.name}`",
        f"> extracted: {today}",
    ]

    with open(extract_path, "w", encoding="utf-8") as f:
        f.write("\n".join(header + content_lines) + "\n")

    # 統計
    image_count = len(list(images_dir.glob("*"))) if images_dir.exists() else 0
    print(f"Extract: {extract_path.name} ({len(content_lines)} lines, {image_count} images)", file=sys.stderr)

    # 輸出路徑供呼叫端使用
    print(str(extract_path))


if __name__ == "__main__":
    main()
