"""extract step of summarize-notes: count the lines of the bound notes into facts.json.

Usage: python .process/scripts/count_lines.py <notes.md> <work-folder>
"""

import json
import pathlib
import sys

text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
pathlib.Path(sys.argv[2], "facts.json").write_text(json.dumps({"lines": len(text.splitlines())}))
