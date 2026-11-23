"""Question text for a ``note`` input source in S4 (SFR-AGG-009). Pure string."""

from ..state import InputSource


def note_question(source: InputSource) -> str:
    need = "required" if source.required else "optional"
    return (f"Input '{source.name}' ({need}) takes a note from you.\n"
            f"{(source.note or '').strip()}\n"
            "Type your note and press Enter.")
