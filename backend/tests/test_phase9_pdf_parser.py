from __future__ import annotations

from app.services.pdf_parser import parse_syllabus_text

SAMPLE = """
UNIT I — Introduction to DBMS
1 Data models
1.1 Hierarchical model
1.2 Network model
1.3 Relational model
2 Database architecture
2.1 Three schema architecture
UNIT II — SQL
3 Queries
3.1 Select
3.2 Joins
Lab 1 — DDL commands
Lab 2 — DML commands
• Insert rows
• Update rows
"""


def test_theory_and_labs_two_branches():
    tree = parse_syllabus_text(SAMPLE)
    names = [c["name"] for c in tree["children"]]
    assert names == ["Theory", "Labs"]
    theory = tree["children"][0]
    labs = tree["children"][1]
    unit_names = [c["name"] for c in theory["children"]]
    assert any("UNIT I" in n for n in unit_names)
    assert any("UNIT II" in n for n in unit_names)
    lab_names = [c["name"] for c in labs["children"]]
    assert any("Lab 1" in n for n in lab_names)
    assert any("Lab 2" in n for n in lab_names)
    lab2 = next(c for c in labs["children"] if "Lab 2" in c["name"])
    bullets = [c["name"] for c in lab2["children"]]
    assert "Insert rows" in bullets
