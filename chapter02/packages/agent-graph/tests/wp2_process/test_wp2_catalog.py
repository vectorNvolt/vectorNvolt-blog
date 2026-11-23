"""SFR-AGG-004: process listing and duplicate ids."""

from agent_graph.process.catalog import duplicate_findings, find_entry, list_processes
from fakes.workspace import add_process, process_with_id


def test_lists_unique_processes(workspace):
    add_process(workspace, "other.yml", process_with_id("other"))
    entries = list_processes(workspace)
    assert [(e.id, e.file, e.selectable) for e in entries] == [
        ("example", ".process/example.yml", True), ("other", ".process/other.yml", True)]
    assert entries[0].title == "Example summary process"
    assert entries[0].description.startswith("Summarizes")
    assert duplicate_findings(entries) == []


def test_shared_id_makes_every_file_unselectable_and_names_files(workspace):
    add_process(workspace, "copy.yml", process_with_id("example"))
    add_process(workspace, "other.yml", process_with_id("other"))
    entries = list_processes(workspace)
    by_file = {e.file: e for e in entries}
    assert not by_file[".process/example.yml"].selectable
    assert by_file[".process/example.yml"].duplicates == (".process/copy.yml",)
    assert by_file[".process/copy.yml"].duplicates == (".process/example.yml",)
    assert by_file[".process/other.yml"].selectable
    fs = duplicate_findings(entries)
    assert len(fs) == 2 and all(f.code == "dup_process_id" for f in fs)
    assert all(".process/copy.yml" in f.message and ".process/example.yml" in f.message for f in fs)
    assert find_entry(entries, "example") is None
    assert find_entry(entries, "other").file == ".process/other.yml"


def test_unreadable_or_idless_files_are_not_selectable(workspace):
    add_process(workspace, "broken.yml", "id: [unclosed\n")
    add_process(workspace, "noid.yaml", "title: x\n")
    entries = {e.file: e for e in list_processes(workspace)}
    for name in (".process/broken.yml", ".process/noid.yaml"):
        assert not entries[name].selectable and entries[name].id == "" and entries[name].error
    assert entries[".process/example.yml"].selectable


def test_no_process_folder(empty_workspace):
    assert list_processes(empty_workspace) == []
