"""Private ZIP/XML verifier. It never executes candidate code or fetches URLs."""
import hashlib
import json
import pathlib
import posixpath
import sys
import urllib.parse
import xml.etree.ElementTree as ET
import zipfile
import zlib

MAX_XML_BYTES = 8 << 20
MAX_EXPANDED_BYTES = 256 << 20
MAX_MEMBERS = 8192
REL_NS = "http://schemas.openxmlformats.org/package/2006/relationships"
DOC_REL_NS = {"http://schemas.openxmlformats.org/officeDocument/2006/relationships",
              "http://purl.oclc.org/ooxml/officeDocument/relationships"}
PRESENTATION_NS = {"http://schemas.openxmlformats.org/presentationml/2006/main",
                   "http://purl.oclc.org/ooxml/presentationml/main"}
DRAWING_NS = {"http://schemas.openxmlformats.org/drawingml/2006/main",
             "http://purl.oclc.org/ooxml/drawingml/main"}
CHART_NS = {"http://schemas.openxmlformats.org/drawingml/2006/chart",
           "http://purl.oclc.org/ooxml/drawingml/chart"}


def tag_is(node, namespaces, name):
    return any(node.tag == "{" + namespace + "}" + name for namespace in namespaces)


def relation_id(node, attribute="id"):
    return next((node.get("{" + namespace + "}" + attribute)
                 for namespace in DOC_REL_NS
                 if node.get("{" + namespace + "}" + attribute)), None)


class Package:
    def __init__(self, archive):
        self.archive = archive
        entries = archive.infolist()
        if len(entries) > MAX_MEMBERS or sum(item.file_size for item in entries) > MAX_EXPANDED_BYTES:
            raise ValueError("PPTX package expansion exceeds verifier limits")
        self.members = {item.filename: item for item in entries}
        if len(self.members) != len(entries):
            raise ValueError("duplicate ZIP member")
        for name in self.members:
            if name.startswith("/") or "\\" in name or ".." in name.split("/"):
                raise ValueError("unsafe ZIP member path")
        if archive.testzip() is not None:
            raise ValueError("ZIP CRC check failed")

    def xml(self, path):
        info = self.members.get(path)
        if info is None or info.file_size > MAX_XML_BYTES:
            raise ValueError("missing or oversized XML part")
        content = self.archive.read(path)
        if b"<!DOCTYPE" in content or b"<!ENTITY" in content:
            raise ValueError("XML declarations are not allowed")
        return ET.fromstring(content)

    def relationships(self, part):
        path = posixpath.join(posixpath.dirname(part), "_rels", posixpath.basename(part) + ".rels") if part else "_rels/.rels"
        if path not in self.members:
            return {}
        root = self.xml(path)
        if root.tag != "{" + REL_NS + "}Relationships":
            raise ValueError("invalid relationships part")
        result = {}
        for node in root:
            identifier = node.get("Id")
            if not identifier or identifier in result:
                raise ValueError("missing or duplicate relationship ID")
            result[identifier] = node
        return result

    def target(self, source, node, kind):
        if node is None or not any(node.get("Type") == namespace + "/" + kind for namespace in DOC_REL_NS):
            raise ValueError("missing required " + kind + " relationship")
        if node.get("TargetMode", "Internal") != "Internal":
            raise ValueError("external " + kind + " does not supply artifact contents")
        target = urllib.parse.urlsplit(node.get("Target", ""))
        if target.scheme or target.netloc or target.query or target.fragment:
            raise ValueError("invalid internal relationship target")
        decoded = urllib.parse.unquote(target.path)
        path = posixpath.normpath(decoded.lstrip("/") if decoded.startswith("/") else posixpath.join(posixpath.dirname(source), decoded))
        if path.startswith("../") or path == ".." or "\\" in path or path not in self.members:
            raise ValueError("missing or unsafe relationship target")
        return path

    def slide(self, path):
        root = self.xml(path)
        if not tag_is(root, PRESENTATION_NS, "sld"):
            raise ValueError("slide relationship does not target a slide")
        content = next((node for node in root if tag_is(node, PRESENTATION_NS, "cSld")), None)
        if content is None:
            raise ValueError("slide has no content tree")
        kinds = set()
        relationships = self.relationships(path)
        for node in content.iter():
            if tag_is(node, DRAWING_NS, "t") and (node.text or "").strip():
                kinds.add("text")
            if tag_is(node, DRAWING_NS, "blip"):
                identifier = relation_id(node, "embed")
                if identifier:
                    image = self.target(path, relationships.get(identifier), "image")
                    if self.members[image].file_size:
                        kinds.add("image")
            if tag_is(node, CHART_NS, "chart"):
                chart = self.xml(self.target(path, relationships.get(relation_id(node)), "chart"))
                if not tag_is(chart, CHART_NS, "chartSpace"):
                    raise ValueError("chart relationship does not target a chart")
                if any((tag_is(item, CHART_NS, "v") or tag_is(item, DRAWING_NS, "t"))
                       and (item.text or "").strip() for item in chart.iter()):
                    kinds.add("chart")
        return {"part": path, "nonempty": bool(kinds), "content_kinds": sorted(kinds)}

    def slides(self):
        types = self.xml("[Content_Types].xml")
        if types.tag != "{http://schemas.openxmlformats.org/package/2006/content-types}Types":
            raise ValueError("invalid content types part")
        documents = [node for node in self.relationships("").values()
                     if any(node.get("Type") == namespace + "/officeDocument" for namespace in DOC_REL_NS)]
        if len(documents) != 1:
            raise ValueError("PPTX must have one presentation document")
        presentation = self.target("", documents[0], "officeDocument")
        expected_type = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
        if not any(node.get("PartName") == "/" + presentation and node.get("ContentType") == expected_type for node in types):
            raise ValueError("package is not a PPTX presentation")
        root = self.xml(presentation)
        if not tag_is(root, PRESENTATION_NS, "presentation"):
            raise ValueError("invalid presentation document")
        slide_list = next((node for node in root if tag_is(node, PRESENTATION_NS, "sldIdLst")), None)
        relationships = self.relationships(presentation)
        slides = []
        seen = set()
        for node in [] if slide_list is None else slide_list:
            if not tag_is(node, PRESENTATION_NS, "sldId"):
                raise ValueError("unexpected slide list entry")
            path = self.target(presentation, relationships.get(relation_id(node)), "slide")
            if path in seen:
                raise ValueError("duplicate slide part in presentation")
            seen.add(path)
            slides.append(self.slide(path))
        return slides


def verify_file(root, item):
    result = {"path": item["path"], "size_bytes": item["size_bytes"], "sha256": item["sha256"],
              "parseable": False, "slide_count": 0, "slides": [], "passed": False, "error": ""}
    try:
        candidate = root / "decks" / item["path"]
        if candidate.is_symlink() or candidate.stat().st_size != item["size_bytes"]:
            raise ValueError("candidate size or type changed after capture")
        digest = hashlib.sha256()
        with candidate.open("rb") as stream:
            for block in iter(lambda: stream.read(1 << 20), b""):
                digest.update(block)
        if digest.hexdigest() != item["sha256"]:
            raise ValueError("candidate SHA-256 changed after capture")
        with zipfile.ZipFile(candidate) as archive:
            slides = Package(archive).slides()
        result.update(parseable=True, slide_count=len(slides), slides=slides,
                      passed=len(slides) == 5 and all(slide["nonempty"] for slide in slides))
        if not result["passed"]:
            result["error"] = "requires exactly 5 slides, each with nonempty body content"
    except (OSError, ValueError, LookupError, ET.ParseError, zipfile.BadZipFile, RuntimeError, NotImplementedError, zlib.error, EOFError) as exc:
        result["error"] = type(exc).__name__ + ": " + str(exc)
    return result


def main():
    root = pathlib.Path(sys.argv[1])
    manifest = json.loads((root / "manifest.json").read_text(encoding="utf-8"))
    if manifest["schema_version"] != 1 or not manifest["complete"]:
        raise ValueError("incomplete artifact inventory")
    for item in manifest["files"]:
        path = pathlib.PurePosixPath(item["path"])
        if path.is_absolute() or ".." in path.parts or str(path) != item["path"]:
            raise ValueError("unsafe artifact path")
    if len(sys.argv) == 3 and sys.argv[2] == "--prepare":
        for item in manifest["files"]:
            (root / "decks" / item["path"]).parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        return 0
    files = [verify_file(root, item) for item in manifest["files"]]
    report = {"schema_version": 1, "passed": any(item["passed"] for item in files), "files": files}
    with (root / "pptx-report.json").open("x", encoding="utf-8") as output:
        json.dump(report, output, ensure_ascii=False, sort_keys=True)
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
