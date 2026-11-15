import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('cache_prune.py')


class CachePruneTest(unittest.TestCase):
    def test_selects_old_go_caches_per_job_and_ref(self):
        def cache(id, job, ref, date, hash_value='a', sha='b'):
            return {'id': id, 'key': f'go-Linux-X64-{job}-{hash_value * 64}-{sha * 40}',
                    'ref': ref, 'created_at': f'2026-11-{date:02}T00:00:00Z', 'size_in_bytes': id * 10}
        caches = [cache(1, 'static', 'refs/heads/main', 1),
                  cache(2, 'static', 'refs/heads/main', 3, 'c', 'd'),
                  cache(3, 'static', 'refs/pull/1/merge', 2),
                  cache(4, 'integration-proof', 'refs/heads/main', 1),
                  cache(5, 'integration-proof', 'refs/heads/main', 2),
                  {'id': 6, 'key': 'unrelated-cache', 'ref': 'refs/heads/main',
                   'created_at': '2026-11-01T00:00:00Z', 'size_in_bytes': 60}]
        with tempfile.TemporaryDirectory() as directory:
            listing = Path(directory) / 'listing.json'
            pages = [caches[:3], [caches[1]] + caches[3:]]
            listing.write_text(''.join(json.dumps({'total_count': len(caches), 'actions_caches': page})
                                       for page in pages))
            result = subprocess.run([sys.executable, str(SCRIPT), 'select', str(listing)],
                                    capture_output=True, text=True, check=True)
        self.assertEqual(result.stdout.splitlines(), ['1', '4'])


if __name__ == '__main__':
    unittest.main()
