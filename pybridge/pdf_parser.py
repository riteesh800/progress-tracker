from __future__ import annotations

import re
from dataclasses import dataclass, field


DECIMAL_LINE = re.compile(r"^\s*(\d+(?:\.\d+)*)\.?\s+(.+)$")
UNIT_LINE = re.compile(r"^\s*(UNIT|Unit|Chapter|CHAPTER)\s+([IVXLC\d]+)(?:\s*[—\-:]\s*|\s+)(.+)$")
LAB_LINE = re.compile(r"^\s*(Lab|LAB|Experiment)\s+(\d+)\s*[—\-:]\s*(.+)$")
BULLET_LINE = re.compile(r"^\s*[•\-\*]\s+(.+)$")


@dataclass
class ProposedNode:
    name: str
    children: list["ProposedNode"] = field(default_factory=list)
    confidence: str = "high"  # high | low
    source: str = "decimal"

    def to_dict(self) -> dict:
        return {
            "name": self.name,
            "confidence": self.confidence,
            "source": self.source,
            "children": [c.to_dict() for c in self.children],
        }


def _depth_from_number(num: str) -> int:
    return len(num.split("."))


def parse_syllabus_text(text: str) -> dict:
    """
    Rule-based structure detection (Part 6.2).
    Returns a proposed tree with optional Theory/Labs split.
    """
    lines = [ln.rstrip() for ln in text.splitlines() if ln.strip()]
    theory_nodes: list[ProposedNode] = []
    lab_nodes: list[ProposedNode] = []
    stack: list[tuple[int, ProposedNode]] = []
    in_labs = False
    low_confidence_sections: list[str] = []
    current_heading: ProposedNode | None = None
    unstructured_buffer: list[str] = []

    def flush_unstructured() -> None:
        nonlocal unstructured_buffer
        if not unstructured_buffer:
            return
        blob = "\n".join(unstructured_buffer)
        unstructured_buffer = []
        if current_heading is not None:
            current_heading.confidence = "low"
            low_confidence_sections.append(blob)
        else:
            low_confidence_sections.append(blob)

    def attach(node: ProposedNode, depth: int, into_labs: bool) -> None:
        nonlocal stack
        target = lab_nodes if into_labs else theory_nodes
        while stack and stack[-1][0] >= depth:
            stack.pop()
        if not stack:
            target.append(node)
        else:
            stack[-1][1].children.append(node)
        stack.append((depth, node))

    for line in lines:
        lab_m = LAB_LINE.match(line)
        if lab_m:
            flush_unstructured()
            in_labs = True
            stack = []
            node = ProposedNode(name=f"Lab {lab_m.group(2)} — {lab_m.group(3).strip()}", source="lab")
            lab_nodes.append(node)
            stack.append((0, node))
            current_heading = node
            continue

        unit_m = UNIT_LINE.match(line)
        if unit_m:
            flush_unstructured()
            stack = []
            node = ProposedNode(
                name=f"{unit_m.group(1)} {unit_m.group(2)} — {unit_m.group(3).strip()}",
                source="unit",
            )
            dest = lab_nodes if in_labs else theory_nodes
            dest.append(node)
            stack.append((0, node))
            current_heading = node
            continue

        dec_m = DECIMAL_LINE.match(line)
        if dec_m:
            flush_unstructured()
            depth = _depth_from_number(dec_m.group(1))
            node = ProposedNode(name=dec_m.group(2).strip(), source="decimal", confidence="high")
            attach(node, depth, in_labs)
            current_heading = node
            continue

        bullet_m = BULLET_LINE.match(line)
        if bullet_m and current_heading is not None:
            flush_unstructured()
            leaf = ProposedNode(name=bullet_m.group(1).strip(), source="bullet", confidence="high")
            current_heading.children.append(leaf)
            continue

        unstructured_buffer.append(line)

    flush_unstructured()

    has_theory = bool(theory_nodes)
    has_labs = bool(lab_nodes)
    if has_theory and has_labs:
        roots = [
            ProposedNode(name="Theory", source="split", children=theory_nodes).to_dict(),
            ProposedNode(name="Labs", source="split", children=lab_nodes).to_dict(),
        ]
    elif has_labs:
        roots = [ProposedNode(name="Labs", source="split", children=lab_nodes).to_dict()]
    else:
        roots = [n.to_dict() for n in theory_nodes]

    return {
        "name": "Imported syllabus",
        "children": roots,
        "low_confidence_sections": low_confidence_sections,
    }
