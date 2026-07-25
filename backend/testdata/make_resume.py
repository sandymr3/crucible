"""Generate a minimal but realistic resume PDF fixture.

Written by hand rather than pulled from a library so the repo gains no build
dependency for one test file. The content is deliberately uneven: some claims
name a mechanism and can be probed deeply, others name only an outcome. That is
what makes it a useful fixture — a resume where every claim is equally strong
tells you nothing about whether the digest is discriminating.

Usage: python testdata/make_resume.py testdata/resume.pdf
"""
import sys
import zlib

LINES = [
    ("H1", "ARJUN MEHTA"),
    ("SUB", "Final-year B.Tech CSE  |  arjun.mehta@example.com  |  github.com/arjunm"),
    ("GAP", ""),
    ("H2", "SKILLS"),
    ("P", "Python, Go, PyTorch, FastAPI, PostgreSQL, Redis, Kafka, Docker, GCP"),
    ("GAP", ""),
    ("H2", "PROJECTS"),
    ("H3", "DataMesh - Streaming feature pipeline"),
    ("P", "Built an async ingestion proxy in Python handling roughly 2000 requests"),
    ("P", "per second across 12 upstream sources. Used one Kafka topic per source"),
    ("P", "and deduplicated downstream with a bloom filter before writing to the"),
    ("P", "feature store. Cut end-to-end feature staleness from 40s to under 5s."),
    ("GAP", ""),
    ("H3", "RecSys-Lite - Retrieval and ranking service"),
    ("P", "Two-stage recommender: ANN candidate retrieval over FAISS, then a"),
    ("P", "gradient-boosted ranker. Served with FastAPI behind Redis caching."),
    ("P", "Improved click-through rate by 18% in an offline replay evaluation."),
    ("GAP", ""),
    ("H3", "ShardKV - Distributed key-value store (course project)"),
    ("P", "Implemented Raft consensus in Go with log compaction and snapshotting."),
    ("P", "Supported dynamic shard rebalancing across replica groups."),
    ("GAP", ""),
    ("H2", "EXPERIENCE"),
    ("H3", "Software Engineering Intern, Netlyx Systems (Summer 2025)"),
    ("P", "Migrated a monolithic billing job to a queue-driven worker pool,"),
    ("P", "reducing nightly batch runtime from 6 hours to 90 minutes."),
    ("P", "Added Prometheus metrics and on-call runbooks for the new pipeline."),
    ("GAP", ""),
    ("H2", "EDUCATION"),
    ("P", "B.Tech Computer Science and Engineering, 2026. CGPA 8.7/10."),
    ("P", "Relevant coursework: Distributed Systems, Machine Learning, Databases."),
]

STYLE = {
    "H1": ("/F2", 20, 26),
    "SUB": ("/F1", 9.5, 16),
    "H2": ("/F2", 12.5, 22),
    "H3": ("/F2", 10.5, 16),
    "P": ("/F1", 10, 14),
    "GAP": ("/F1", 10, 8),
}


def escape(text: str) -> str:
    return text.replace("\\", r"\\").replace("(", r"\(").replace(")", r"\)")


def build_content() -> bytes:
    out = ["BT", "1 0 0 1 62 780 Tm"]
    for kind, text in LINES:
        font, size, leading = STYLE[kind]
        out.append(f"{font} {size} Tf")
        out.append(f"0 -{leading} Td")
        if text:
            out.append(f"({escape(text)}) Tj")
    out.append("ET")
    return "\n".join(out).encode("latin-1")


def build_pdf() -> bytes:
    stream = zlib.compress(build_content())

    objects = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
        b"/Resources << /Font << /F1 5 0 R /F2 6 0 R >> >> /Contents 4 0 R >>",
        b"<< /Length " + str(len(stream)).encode() + b" /Filter /FlateDecode >>\nstream\n"
        + stream + b"\nendstream",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>",
    ]

    pdf = bytearray(b"%PDF-1.4\n")
    offsets = []
    for i, body in enumerate(objects, start=1):
        offsets.append(len(pdf))
        pdf += f"{i} 0 obj\n".encode() + body + b"\nendobj\n"

    xref_pos = len(pdf)
    pdf += f"xref\n0 {len(objects) + 1}\n".encode()
    pdf += b"0000000000 65535 f \n"
    for off in offsets:
        pdf += f"{off:010d} 00000 n \n".encode()
    pdf += (
        f"trailer\n<< /Size {len(objects) + 1} /Root 1 0 R >>\n"
        f"startxref\n{xref_pos}\n%%EOF\n"
    ).encode()
    return bytes(pdf)


if __name__ == "__main__":
    path = sys.argv[1] if len(sys.argv) > 1 else "testdata/resume.pdf"
    with open(path, "wb") as fh:
        fh.write(build_pdf())
    print(f"wrote {path}")
