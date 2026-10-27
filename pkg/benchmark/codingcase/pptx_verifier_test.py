"""Trusted fixtures only; this is not an Agent or cloud execution result."""
import hashlib
import json
import pathlib
import subprocess
import sys
import tempfile
import unittest
import zipfile

HERE = pathlib.Path(__file__).resolve().parent
P = "http://schemas.openxmlformats.org/presentationml/2006/main"
A = "http://schemas.openxmlformats.org/drawingml/2006/main"
C = "http://schemas.openxmlformats.org/drawingml/2006/chart"
R = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
PKG = "http://schemas.openxmlformats.org/package/2006/relationships"


def relationships(items):
    return '<Relationships xmlns="' + PKG + '">' + ''.join(
        '<Relationship Id="' + identifier + '" Type="' + R + '/' + kind + '" Target="' + target + '"/>'
        for identifier, kind, target in items) + '</Relationships>'


def make_deck(target, count=5, last="text"):
    target.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(target, "w") as archive:
        archive.writestr("[Content_Types].xml", '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/></Types>')
        archive.writestr("_rels/.rels", relationships([("root", "officeDocument", "ppt/presentation.xml")]))
        archive.writestr("ppt/presentation.xml", '<p:presentation xmlns:p="' + P + '" xmlns:r="' + R + '"><p:sldIdLst>' + ''.join('<p:sldId id="' + str(256 + i) + '" r:id="s' + str(i) + '"/>' for i in range(count)) + '</p:sldIdLst></p:presentation>')
        archive.writestr("ppt/_rels/presentation.xml.rels", relationships([("s" + str(i), "slide", "slides/s" + str(i) + ".xml") for i in range(count)]))
        for index in range(count):
            kind = last if index == count - 1 else "text"
            body = "<p:sp><p:txBody><a:p><a:r><a:t>大模型发展</a:t></a:r></a:p></p:txBody></p:sp>"
            if kind in ("empty", "notes", "unreferenced"):
                body = "<p:sp><p:txBody><a:p><a:r><a:t> \n </a:t></a:r></a:p></p:txBody></p:sp>"
            if kind in ("image", "broken-image"):
                body = '<p:pic><p:blipFill><a:blip r:embed="asset"/></p:blipFill></p:pic>'
                archive.writestr("ppt/slides/_rels/s" + str(index) + ".xml.rels", relationships([("asset", "image", "../media/picture.png")]))
            if kind == "chart":
                body = '<p:graphicFrame><a:graphic><a:graphicData><c:chart r:id="asset"/></a:graphicData></a:graphic></p:graphicFrame>'
                archive.writestr("ppt/slides/_rels/s" + str(index) + ".xml.rels", relationships([("asset", "chart", "../charts/chart.xml")]))
                archive.writestr("ppt/charts/chart.xml", '<c:chartSpace xmlns:c="' + C + '"><c:chart><c:plotArea><c:barChart><c:ser><c:val><c:numLit><c:pt idx="0"><c:v>1</c:v></c:pt></c:numLit></c:val></c:ser></c:barChart></c:plotArea></c:chart></c:chartSpace>')
            if kind in ("image", "unreferenced"):
                archive.writestr("ppt/media/picture.png", b"\x89PNG\r\n\x1a\nfixture-image-content")
            if kind == "notes":
                archive.writestr("ppt/notesSlides/notes1.xml", '<p:notes xmlns:p="' + P + '" xmlns:a="' + A + '"><a:t>Notes alone cannot pass</a:t></p:notes>')
            archive.writestr("ppt/slides/s" + str(index) + ".xml", '<p:sld xmlns:p="' + P + '" xmlns:a="' + A + '" xmlns:c="' + C + '" xmlns:r="' + R + '"><p:cSld><p:spTree>' + body + '</p:spTree></p:cSld></p:sld>')


class PPTXVerifierTest(unittest.TestCase):
    def verify(self, specifications):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            files = []
            for name, options in specifications:
                candidate = root / "decks" / name
                if options == "corrupt":
                    candidate.parent.mkdir(parents=True, exist_ok=True)
                    candidate.write_bytes(b"this is not a ZIP")
                elif options == "zero":
                    candidate.parent.mkdir(parents=True, exist_ok=True)
                    candidate.write_bytes(b"")
                else:
                    make_deck(candidate, **options)
                files.append({"path": name, "size_bytes": candidate.stat().st_size,
                              "sha256": hashlib.sha256(candidate.read_bytes()).hexdigest()})
            (root / "manifest.json").write_text(json.dumps({"schema_version": 1, "complete": True, "files": files}))
            process = subprocess.run([sys.executable, "-I", "-B", str(HERE / "pptx_verifier.py"), str(root)], capture_output=True, text=True, timeout=10)
            self.assertIn(process.returncode, (0, 1), process.stderr)
            report = json.loads((root / "pptx-report.json").read_text())
            self.assertEqual(process.returncode == 0, report["passed"])
            self.assertEqual(len(files), len(report["files"]))
            return report

    def test_five_nonempty_slides(self):
        report = self.verify([("输出/大模型发展.pptx", {})])
        self.assertTrue(report["passed"])
        self.assertEqual(report["files"][0]["slide_count"], 5)

    def test_wrong_count(self):
        self.assertFalse(self.verify([("four.pptx", {"count": 4})])["passed"])

    def test_empty_slide(self):
        self.assertFalse(self.verify([("empty.pptx", {"last": "empty"})])["passed"])

    def test_corrupt_and_empty_files_remain_visible(self):
        report = self.verify([("broken.pptx", "corrupt"), ("zero.pptx", "zero")])
        self.assertFalse(report["passed"])
        self.assertTrue(all(not item["parseable"] and item["error"] for item in report["files"]))

    def test_no_files(self):
        self.assertFalse(self.verify([])["passed"])

    def test_passing_final_does_not_hide_failed_intermediate(self):
        report = self.verify([("draft.pptx", "corrupt"), ("final.pptx", {})])
        self.assertTrue(report["passed"])
        self.assertFalse(report["files"][0]["passed"])
        self.assertTrue(report["files"][1]["passed"])

    def test_referenced_image_and_chart(self):
        for kind in ("image", "chart"):
            with self.subTest(kind=kind):
                report = self.verify([("visual.pptx", {"last": kind})])
                self.assertTrue(report["passed"])
                self.assertEqual(report["files"][0]["slides"][-1]["content_kinds"], [kind])

    def test_notes_unreferenced_image_and_broken_reference_do_not_pass(self):
        for kind in ("notes", "unreferenced", "broken-image"):
            with self.subTest(kind=kind):
                self.assertFalse(self.verify([("empty.pptx", {"last": kind})])["passed"])

    def test_capture_exports_all_names_and_does_not_silently_skip_symlinks(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            workspace = root / "workspace"
            make_deck(workspace / "输出" / "大模型发展.PPTX")
            (workspace / "empty.pptx").write_bytes(b"")
            capture = root / "capture.json"
            result = subprocess.run([sys.executable, "-I", "-B", str(HERE / "pptx_capture.py"), str(workspace), str(capture)], capture_output=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
            files = json.loads(capture.read_text())["files"]
            self.assertEqual([item["path"] for item in files], ["empty.pptx", "输出/大模型发展.PPTX"])
            (workspace / "link.pptx").symlink_to(workspace / "empty.pptx")
            capture.unlink()
            result = subprocess.run([sys.executable, "-I", "-B", str(HERE / "pptx_capture.py"), str(workspace), str(capture)], capture_output=True, timeout=10)
            self.assertEqual(result.returncode, 1)
            self.assertFalse(json.loads(capture.read_text())["complete"])

    def test_capture_size_bound_is_an_explicit_failure(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            workspace = root / "workspace"
            workspace.mkdir()
            with (workspace / "too-big.pptx").open("wb") as output:
                output.truncate((64 << 20) + 1)
            capture = root / "capture.json"
            result = subprocess.run([sys.executable, "-I", "-B", str(HERE / "pptx_capture.py"), str(workspace), str(capture)], capture_output=True, timeout=10)
            self.assertEqual(result.returncode, 1)
            report = json.loads(capture.read_text())
            self.assertFalse(report["complete"])
            self.assertIn("limit", report["error"])

    def test_capture_count_boundary_preserves_all_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            workspace = root / "workspace"
            workspace.mkdir()
            for index in range(128):
                (workspace / f"{index:03}.pptx").touch()
            capture = root / "capture.json"
            command = [sys.executable, "-I", "-B", str(HERE / "pptx_capture.py"), str(workspace), str(capture)]
            result = subprocess.run(command, capture_output=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
            report = json.loads(capture.read_text())
            self.assertTrue(report["complete"])
            self.assertEqual(len(report["files"]), 128)
            (workspace / "129.pptx").touch()
            capture.unlink()
            result = subprocess.run(command, capture_output=True, timeout=10)
            self.assertEqual(result.returncode, 1)
            self.assertFalse(json.loads(capture.read_text())["complete"])

    def test_capture_total_boundary_preserves_every_original(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            workspace = root / "workspace"
            workspace.mkdir()
            for index in range(8):
                with (workspace / f"{index}.pptx").open("wb") as output:
                    output.truncate(64 << 20)
            capture = root / "capture.json"
            command = [sys.executable, "-I", "-B", str(HERE / "pptx_capture.py"), str(workspace), str(capture)]
            result = subprocess.run(command, capture_output=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stderr)
            report = json.loads(capture.read_text())
            self.assertTrue(report["complete"])
            self.assertEqual(len(report["files"]), 8)
            self.assertEqual(sum(item["size_bytes"] for item in report["files"]), 512 << 20)
            (workspace / "extra.pptx").write_bytes(b"x")
            capture.unlink()
            result = subprocess.run(command, capture_output=True, timeout=30)
            self.assertEqual(result.returncode, 1)
            self.assertFalse(json.loads(capture.read_text())["complete"])


if __name__ == "__main__":
    unittest.main()
