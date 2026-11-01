import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]

class GlobalToggleTests(unittest.TestCase):
    def test_global_toggle_never_navigates_or_uses_repo_context(self):
        with tempfile.TemporaryDirectory() as temp:
            base = Path(temp)
            cli = base / "herdr"
            log = base / "calls.jsonl"
            cli.write_text('#!/usr/bin/env python3\nimport os,sys,json\nwith open(os.environ["TEST_LOG"],"a") as f: f.write(json.dumps(sys.argv[1:])+"\\n")\n')
            cli.chmod(0o700)
            for workspace in ["w1", "w2", None]:
                env = dict(os.environ, HERDR_BIN_PATH=str(cli), TEST_LOG=str(log), XDG_DATA_HOME=str(base / "user data"), HERDR_PLUGIN_CONTEXT_JSON=json.dumps({"workspace_id":workspace,"focused_pane_cwd":"/unrelated/repo"}))
                result = subprocess.run(["/bin/sh",str(ROOT / "scripts/global-toggle.sh")], env=env, capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            self.assertEqual(calls, [["plugin","runtime","toggle","--plugin","herdr-postman","--entrypoint","postman","--cwd",str(base / "user data/herdr-postman")]] * 3)
