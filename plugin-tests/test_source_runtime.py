"""Source switching in a real PTY using only a fixture Postman CLI."""
import fcntl
import json
import hashlib
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import tempfile
import termios
import time
import unittest

BINARY = Path(os.environ.get("HERDR_POSTMAN_TEST_BINARY", str(Path(__file__).resolve().parents[1] / "bin/herdr-postman")))

@unittest.skipUnless(BINARY.exists(), 'build bin/herdr-postman first')
class SourceRuntime(unittest.TestCase):
    def test_cloud_and_local_can_be_selected_in_one_process(self):
        with tempfile.TemporaryDirectory(prefix='hpm-source-') as temp:
            root = Path(temp)
            local = root/'local'
            local.mkdir()
            (local/'get.request.yaml').write_text('$kind: http-request\nname: Local fixture\nmethod: GET\nurl: https://example.test\n')
            cli = root/'postman'
            cli.write_text('''#!/bin/sh
case "$*" in
 'environment list --workspace w1 --json') printf '%s' '[]' ;;
 'workspace list --json') printf '%s' '{"workspaces":[{"id":"w1","name":"Fixture workspace"}]}' ;;
 'collection list --workspace w1 --json') printf '%s' '[{"id":"c1","name":"Fixture Cloud A"},{"id":"c2","name":"Fixture Cloud B"}]' ;;
 'collection get c2 --json') printf '%s' '{"$kind":"collection","id":"c2","name":"Fixture Cloud B","items":[]}' ;;
 'collection get c1 --json') printf '%s' '{"$kind":"collection","id":"c1","name":"Fixture Cloud A","items":[{"$kind":"http-request","id":"r1","name":"Child fixture","method":"GET","url":"https://example.test"}]}' ;;
 *) exit 3 ;;
esac
''')
            cli.chmod(0o700)
            config = root/'config.yaml'
            config.write_text('history:\n  enabled: false\nremote_control:\n  enabled: false\n')
            master, slave = pty.openpty()
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 140, 0, 0))
            env = dict(os.environ, TERM='xterm-256color', HERDR_POSTMAN_CONFIG_FILE=str(config), XDG_CONFIG_HOME=str(root/'config'), XDG_DATA_HOME=str(root/'data'))
            def launch(local_path=local):
                return subprocess.Popen([str(BINARY), '--source', 'choose', '--collection', str(local_path), '--postman-bin', str(cli)], stdin=slave, stdout=slave, stderr=slave, env=env)
            process = launch()
            def expect(text):
                data = b''
                deadline = time.monotonic()+15
                while time.monotonic()<deadline:
                    if select.select([master], [], [], .1)[0]:
                        try: data += os.read(master, 65536)
                        except OSError: break
                        if text.encode() in data: return
                    if process.poll() is not None: break
                self.fail('Expected terminal state: '+text+'; output: '+repr(data[-4000:]))
            def wait_exit():
                deadline = time.monotonic()+10
                while process.poll() is None and time.monotonic()<deadline:
                    if select.select([master], [], [], .1)[0]:
                        try: os.read(master, 65536)
                        except OSError: break
                result = process.wait(timeout=1)
                while select.select([master], [], [], 0)[0]:
                    try: os.read(master, 65536)
                    except OSError: break
                return result
            try:
                expect('[Local]')
                os.write(master, b'\x1bs')  # Alt+S focuses the source select.
                time.sleep(.15)
                os.write(master, b'\r\x1b[B\r')
                expect('Choose a workspace above')
                os.write(master, b'\r')
                expect('Fixture workspace')
                os.write(master, b'\r')
                expect('Fixture Cloud B')
                os.write(master, b'\x07')  # Search sidebar, then focus its first folder.
                time.sleep(.1)
                os.write(master, b'\r')
                time.sleep(.1)
                os.write(master, b'\x1b[D')  # Move from first request to its collection.
                time.sleep(.1)
                os.write(master, b'\x1b[D')  # Collapse collection A.
                time.sleep(.1)
                os.write(master, b'\x03')
                self.assertEqual(wait_exit(), 0)
                saved = json.loads((root/'data/herdr-postman/source.json').read_text())
                self.assertEqual(set(saved), {'mode', 'workspace', 'localRoot'})
                self.assertEqual(saved['workspace'], 'w1')
                sidebar_file = root/'data/herdr-postman/sidebar'/ (hashlib.sha256(b'cloud\x00w1').hexdigest()+'.json')
                collapsed = ['collection-'+hashlib.sha256(b'c1').hexdigest()]
                self.assertEqual(json.loads(sidebar_file.read_text()), collapsed)
                process = launch()
                expect('Fixture Cloud B')  # Restored automatically, no selection keys.
                os.write(master, b'\x1bs')
                time.sleep(.15)
                os.write(master, b'\r\x1b[A\r')
                expect('Local fixture')
                self.assertEqual(json.loads(sidebar_file.read_text()), collapsed)
                os.write(master, b'\x03')
                self.assertEqual(wait_exit(), 0)
                other = root/'other'; other.mkdir()
                process = launch(other)
                expect('Local fixture')  # Remembered path overrides launch discovery.
                os.write(master, b'\x03')
                self.assertEqual(wait_exit(), 0)
            finally:
                if process.poll() is None:
                    process.terminate()
                    wait_exit()
                os.close(master)
                os.close(slave)
