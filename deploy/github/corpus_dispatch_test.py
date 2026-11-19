import ast
from pathlib import Path
import unittest

import corpus_production
import corpus_upload


class CorpusDispatchTest(unittest.TestCase):
    def test_dispatch_command_names_all_four_inputs(self):
        import corpus_dispatch
        self.assertEqual(corpus_dispatch.dispatch_command('vn', '20990102_1', 'a' * 64, ''),
                         ['gh', 'workflow', 'run', 'corpus-release.yml', '--ref', 'main', '-f', 'country=vn',
                          '-f', 'rel=20990102_1', '-f', 'report_sha256=' + 'a' * 64, '-f', 'release_sha256='])

    def test_upload_and_resign_check_share_a_leaf_module(self):
        import corpus_dispatch
        tree = ast.parse(Path(corpus_dispatch.__file__).read_text())
        self.assertEqual([node for node in ast.walk(tree) if isinstance(node, (ast.Import, ast.ImportFrom))], [])
        self.assertIs(corpus_upload.dispatch_command, corpus_dispatch.dispatch_command)
        self.assertIs(corpus_production.dispatch_command, corpus_dispatch.dispatch_command)
        production = ast.parse(Path(corpus_production.__file__).read_text())
        imported = {node.module for node in ast.walk(production) if isinstance(node, ast.ImportFrom)}
        imported |= {alias.name for node in ast.walk(production) if isinstance(node, ast.Import) for alias in node.names}
        self.assertNotIn('corpus_upload', imported)


if __name__ == '__main__':
    unittest.main()
