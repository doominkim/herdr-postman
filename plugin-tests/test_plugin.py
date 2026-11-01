import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]

class PluginTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='herdr-postman-plugin-')
        self.addCleanup(self.temp.cleanup)
        self.dir = Path(self.temp.name).resolve()
        self.log = self.dir / 'calls.jsonl'
        self.app = self.dir / 'fake app'
        self.app.write_text('#!/usr/bin/env python3\nimport os,json,sys\nwith open(os.environ["TEST_LOG"],"a") as f: f.write(json.dumps(sys.argv[1:])+"\\n")\n')
        self.app.chmod(0o700)
        self.env = dict(os.environ, HERDR_POSTMAN_BIN=str(self.app), POSTMAN_BIN=str(self.app), TEST_LOG=str(self.log), HERDR_POSTMAN_STATE_DIR=str(self.dir/'state'), XDG_DATA_HOME=str(self.dir/'data'))

    def test_marketplace_build_hook_builds_app_from_plugin_root(self):
        import tomllib
        manifest = tomllib.loads((ROOT / 'herdr-plugin.toml').read_text())
        self.assertEqual(manifest['build'][0]['command'], ['/bin/sh', 'scripts/build.sh'])
        fake_go = self.dir / 'go'
        fake_go.write_text('#!/usr/bin/env python3\nimport os,json,sys\nwith open(os.environ["TEST_LOG"],"a") as f: f.write(json.dumps([os.getcwd(),*sys.argv[1:]])+"\\n")\n')
        fake_go.chmod(0o700)
        result = subprocess.run(['/bin/sh', str(ROOT / 'scripts/build.sh')], cwd=self.dir,
                                env=dict(self.env, PATH=str(self.dir)+os.pathsep+os.environ['PATH']), capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls(), [[str(ROOT), 'build', '-o', 'bin/herdr-postman', './cmd/herdr-postman']])

    def pane(self, **extra):
        return subprocess.run(['/bin/sh', str(ROOT/'scripts/pane.sh')], cwd=self.dir, env=dict(self.env, **extra), capture_output=True, text=True)

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_explicit_collection_and_environment_keep_spaces(self):
        collection = self.dir / 'My collection'; collection.mkdir()
        environment = self.dir / 'Dev env.environment.yaml'; environment.write_text('name: Dev\nvalues: []\n')
        result = self.pane(HERDR_POSTMAN_COLLECTION=str(collection), HERDR_POSTMAN_ENVIRONMENT=str(environment))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls(), [['--collection', str(collection), '--postman-bin', str(self.app), '--environment', str(environment), '--source', 'choose']])

    def test_repo_collection_does_not_replace_global_default(self):
        collection = self.dir/'postman/collections/sample'; collection.mkdir(parents=True)
        result = self.pane()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls()[0][1], str(self.dir/'data/herdr-postman/postman/collections/default'))

    def test_no_collection_opens_personal_default(self):
        data = self.dir/'user data'
        result = self.pane(XDG_DATA_HOME=str(data))
        self.assertEqual(result.returncode, 0, result.stderr)
        collection = data/'herdr-postman/postman/collections/default'
        self.assertEqual(self.calls()[0][1], str(collection))
        self.assertTrue((collection/'.resources/definition.yaml').is_file())
        self.assertFalse((self.dir/'postman').exists())

    def test_multiple_local_collections_still_allow_source_selection(self):
        for name in ['one', 'two']:
            (self.dir/'postman/collections'/name).mkdir(parents=True)
        result = self.pane(XDG_DATA_HOME=str(self.dir/'data'))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('herdr-postman/postman/collections/default', self.calls()[0][1])
        self.assertEqual(self.calls()[0][-2:], ['--source', 'choose'])

    def test_default_launcher_offers_both_sources(self):
        result = self.pane(XDG_DATA_HOME=str(self.dir/"data"))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls()[0][-2:], ["--source", "choose"])

    def test_missing_binary_fails_without_launch(self):
        result = self.pane(HERDR_POSTMAN_BIN=str(self.dir/'missing'))
        self.assertEqual(result.returncode, 127)
        self.assertEqual(self.calls(), [])

    def toggle(self, panes, focused, workspace="w1"):
        herdr = self.dir/'herdr'
        herdr.write_text('#!/usr/bin/env python3\nimport os,json,sys\nif sys.argv[1:]==["pane","list"]: print(json.dumps({"result":{"panes":json.loads(os.environ["TEST_PANES"])}}))\nelse:\n with open(os.environ["TEST_LOG"],"a") as f: f.write(json.dumps(sys.argv[1:])+"\\n")\n if sys.argv[1:4]==["plugin","pane","open"]: print(json.dumps({"result":{"plugin_pane":{"pane":{"tab_id":"w1:t9"}}}}))\n')
        herdr.chmod(0o700)
        context = json.dumps(dict(workspace_id=workspace, focused_pane_id=focused, focused_pane_cwd=str(self.dir)))
        return subprocess.run(['/bin/sh', str(ROOT/'scripts/toggle.sh')], env=dict(self.env, HERDR_BIN_PATH=str(herdr), HERDR_PLUGIN_CONTEXT_JSON=context, TEST_PANES=json.dumps(panes)), capture_output=True, text=True)

    def test_toggle_opens_separate_plugin_and_ignores_posting(self):
        result=self.toggle([dict(workspace_id='w1',pane_id='w1:p1',tab_id='w1:t1',label='Posting')], 'w1:p1')
        self.assertEqual(result.returncode,0,result.stderr)
        self.assertEqual(self.calls()[0],['plugin','pane','open','--plugin','herdr-postman','--entrypoint','postman','--placement','tab','--focus','--cwd',str(self.dir/'data/herdr-postman')])
        self.assertEqual(self.calls()[1],['tab','focus','w1:t9'])

    def test_toggle_focuses_then_returns_without_closing(self):
        panes=[dict(workspace_id='w1',pane_id='w1:p1',tab_id='w1:t1',label='shell'),dict(workspace_id='w1',pane_id='w1:p9',tab_id='w1:t9',label='herdr-postman')]
        result=self.toggle(panes,'w1:p1');self.assertEqual(result.returncode,0,result.stderr)
        result=self.toggle(panes,'w1:p9');self.assertEqual(result.returncode,0,result.stderr)
        self.assertEqual(self.calls(),[['tab','focus','w1:t9'],['plugin','pane','focus','w1:p9'],['tab','focus','w1:t1']])

    def test_other_workspace_reuses_global_postman_and_returns_to_caller(self):
        panes=[dict(workspace_id='w1',pane_id='w1:p1',tab_id='w1:t1',label='shell'),dict(workspace_id='w2',pane_id='w2:p9',tab_id='w2:t9',label='herdr-postman')]
        result=self.toggle(panes,'w1:p1')
        self.assertEqual(result.returncode,0,result.stderr)
        result=self.toggle(panes,'w2:p9', workspace='w2')
        self.assertEqual(result.returncode,0,result.stderr)
        self.assertEqual(self.calls(),[['tab','focus','w2:t9'],['plugin','pane','focus','w2:p9'],['tab','focus','w1:t1']])

    def test_existing_duplicate_tabs_are_not_closed_or_used_as_return_target(self):
        panes=[dict(workspace_id='w1',pane_id='w1:p9',tab_id='w1:t9',label='herdr-postman'),dict(workspace_id='w2',pane_id='w2:p9',tab_id='w2:t9',label='herdr-postman'),dict(workspace_id='w2',pane_id='w2:p1',tab_id='w2:t1',label='shell')]
        result=self.toggle(panes,'w2:p9', workspace='w2')
        self.assertEqual(result.returncode,0,result.stderr)
        self.assertEqual(self.calls(),[['tab','focus','w2:t1']])

    def test_only_postman_tab_does_not_close(self):
        result=self.toggle([dict(workspace_id='w1',pane_id='w1:p9',tab_id='w1:t9',label='herdr-postman')], 'w1:p9')
        self.assertEqual(result.returncode,0,result.stderr)
        self.assertEqual(self.calls(),[])

    def test_manifest_has_separate_ids_and_existing_entrypoints(self):
        import tomllib
        manifest=tomllib.loads((ROOT/'herdr-plugin.toml').read_text())
        self.assertEqual(manifest['id'],'herdr-postman')
        self.assertEqual(manifest['panes'][0]['id'],'postman')
        self.assertEqual(manifest['panes'][0]['title'],'herdr-postman')
        for script in ['pane.sh','toggle.sh']:
            result=subprocess.run(['/bin/sh','-n',str(ROOT/'scripts'/script)],capture_output=True,text=True)
            self.assertEqual(result.returncode,0,result.stderr)

if __name__ == '__main__':
    unittest.main()
