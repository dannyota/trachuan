import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import repositories


def enable_dual_policy():
    directory = tempfile.TemporaryDirectory()
    unittest.addModuleCleanup(directory.cleanup)
    path = Path(directory.name) / 'deploy-policy.json'
    value = json.loads(repositories.POLICY.read_bytes())
    value['repositories'] = ['dannyota/trachuan', 'dntmakers/trachuan']
    path.write_text(json.dumps(value))
    patcher = patch.object(repositories, 'POLICY', path)
    patcher.start()
    unittest.addModuleCleanup(patcher.stop)
