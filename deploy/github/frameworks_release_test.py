import importlib
import tempfile
import unittest
from pathlib import Path

from frameworks_test_support import PREVIOUS, REL, STEPS, digest, rebind_build, state, write_objects


class FrameworksReleaseChecksTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('frameworks_release')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.value = state()

    def check(self, value=None, rel=REL):
        return self.module.validate_release(rel, write_objects(self.path / 'objects', value or self.value))

    def test_valid_release_returns_bound_facts(self):
        facts = self.check()
        self.assertEqual(facts.dump_sha256, digest(self.value['dump']))
        self.assertEqual(facts.dump_size, len(self.value['dump']))
        self.assertEqual(facts.row['version_count'], 3)
        self.assertIs(type(facts.row['control_count']), int)
        self.assertIsNone(facts.report['previous'])

    def refuse(self, change):
        value = state()
        change(value)
        with self.assertRaises(ValueError):
            self.check(value)

    def test_refuses_each_listed_mismatch(self):
        changes = {
            'quality false': lambda v: v['report'].update(quality_pass=False),
            'quality string': lambda v: v['report'].update(quality_pass='true'),
            'quality missing': lambda v: v['report'].pop('quality_pass'),
            'report dump hash': lambda v: v['report'].update(dump_sha256='0' * 64),
            'dump bytes': lambda v: v.update(dump=b'other'),
            'build dump hash': lambda v: (v['build']['dump'].update(sha256='0' * 64), rebind_build(v)),
            'build dump missing': lambda v: (v['build'].pop('dump'), rebind_build(v)),
            'build manifest hash': lambda v: v['report'].update(build_manifest_sha256='0' * 64),
            'report rel': lambda v: v['report'].update(rel='20990103'),
            'row rel': lambda v: v['row'].update(rel='20990103'),
            'row kind': lambda v: v['row'].update(kind='law'),
            'build recipe': lambda v: v['report']['recipe_hashes'].update(build='0' * 64),
            'evaluate recipe': lambda v: v['report']['recipe_hashes'].update(evaluate='0' * 64),
            'missing hashes': lambda v: v['report'].pop('recipe_hashes'),
        }
        for name, change in changes.items():
            with self.subTest(name=name):
                self.refuse(change)

    def test_refuses_each_step_recipe_difference_from_the_row(self):
        for step in STEPS:
            with self.subTest(step=step):
                self.refuse(lambda v: v['report']['recipe_hashes'].update({step: '0' * 64}))
            with self.subTest(step=step, side='row'):
                self.refuse(lambda v: v['row'][step + '_recipe']['parameters'].update(changed=True))
            with self.subTest(step=step, side='missing'):
                self.refuse(lambda v: v['row'].pop(step + '_recipe'))

    def test_build_and_evaluate_recipes_bind_their_own_files(self):
        self.refuse(lambda v: v['evaluate']['recipe'].update(version=2))
        value = state()
        value['build']['recipe']['version'] = 2
        rebind_build(value)
        with self.assertRaises(ValueError):
            self.check(value)

    def test_previous_must_be_null_or_a_closed_reference(self):
        good = state()
        good['report']['previous'] = dict(rel=PREVIOUS, report_sha256='b' * 64)
        self.assertEqual(self.check(good).report['previous']['rel'], PREVIOUS)
        for previous in ('20990101', [], dict(rel=PREVIOUS), dict(rel='bad', report_sha256='b' * 64),
                         dict(rel=PREVIOUS, report_sha256='B' * 64),
                         dict(rel=PREVIOUS, report_sha256='b' * 64, extra=1)):
            with self.subTest(previous=previous):
                self.refuse(lambda v: v['report'].update(previous=previous))
        self.refuse(lambda v: v['report'].pop('previous'))

    def test_requested_release_must_match_and_be_valid(self):
        with self.assertRaises(ValueError):
            self.check(rel='20990103')
        with self.assertRaises(ValueError):
            self.check(rel='../x')

    def test_object_set_size_and_json_are_enforced(self):
        paths = write_objects(self.path / 'objects', self.value)
        with self.assertRaises(ValueError):
            self.module.validate_release(REL, {name: path for name, path in paths.items() if name != 'build.json'})
        paths['report.json'].write_bytes(b'{"rel":')
        with self.assertRaises(ValueError):
            self.module.validate_release(REL, paths)
        paths['report.json'].write_bytes(b'')
        with self.assertRaises(ValueError):
            self.module.validate_release(REL, paths)
        with paths['release.json'].open('wb') as stream:
            stream.truncate(16 * 1024**2 + 1)
        with self.assertRaises(ValueError):
            self.module.validate_release(REL, paths)

    def test_locations_and_limits_match_the_design(self):
        self.assertEqual(self.module.BUCKET, 'trachuan-frameworks-releases')
        self.assertEqual(self.module.object_key('20990102_1', 'report.json'), 'frameworks/20990102_1/report.json')
        self.assertEqual(self.module.LIMITS, {'frameworks.dump': 16 * 1024**3, 'build.json': 16 * 1024**2,
                                              'evaluate.json': 16 * 1024**2, 'report.json': 16 * 1024**2,
                                              'release.json': 16 * 1024**2})
        for rel in ('20990102', '20990102_1', '20990102_12'):
            self.module.release_id(rel)
        for rel in ('2099010', '20990102_', '20990102_123', '20990102\n', 'x'):
            with self.subTest(rel=rel), self.assertRaises(ValueError):
                self.module.release_id(rel)


if __name__ == '__main__':
    unittest.main()
