import base64
import copy
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

from updater import deploy_verify as v
from updater.deploy_io import canonical, CommandError


FIXTURES = Path(__file__).with_name('fixtures')
POLICY = Path(__file__).parents[1] / 'deploy-policy.json'


def archive(server=b'GOOGLE_CLIENT_ID=public\n', web=b'NITRO_HOST=127.0.0.1\n'):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode='w:gz') as tar:
        for name, data in [('server.env', server), ('web.env', web), ('install.sh', b'#!/bin/bash\n'),
                           ('quadlet/trachuan-postgres.container', b'Image=@POSTGRES_IMAGE@\n')]:
            entry = tarfile.TarInfo('deploy/host/' + name)
            entry.size = len(data)
            entry.mode = 0o755 if name.endswith('.sh') else 0o644
            tar.addfile(entry, io.BytesIO(data))
    return output.getvalue()


def statement(manifest, image):
    return {'_type': 'https://in-toto.io/Statement/v1',
            'predicateType': 'https://slsa.dev/provenance/v1',
            'subject': [{'name': image.split('@')[0], 'digest': {'sha256': image.split(':')[-1]}}],
            'predicate': {'buildDefinition': {
                'externalParameters': {'workflow': {'ref': manifest['ref'],
                    'repository': 'https://github.com/dannyota/trachuan',
                    'path': '.github/workflows/images.yml'}},
                'resolvedDependencies': [{'uri': 'git+https://github.com/dannyota/trachuan@' + manifest['ref'],
                                          'digest': {'gitCommit': manifest['commit']}}]}}}


class VerificationTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.path = Path(self.work.name)
        self.manifest = json.loads((FIXTURES / 'manifest.json').read_bytes())
        self.policy = v.policy(POLICY)
        self.calls = []

    def run_command(self, args, **kwargs):
        self.calls.append(args)
        if 'image' in args and 'inspect' in args:
            return canonical([{'RepoDigests': [args[-1]]}])
        if 'verify-attestation' in args:
            return canonical({'payload': base64.b64encode(canonical(statement(self.manifest, args[-1]))).decode()})
        return b''

    def test_real_fixture_is_canonical_and_signature_bundle_is_parsed(self):
        self.assertEqual(v.release((FIXTURES / 'manifest.json').read_bytes(), self.policy), self.manifest)
        bundle = json.loads((FIXTURES / 'manifest.sigstore.json').read_bytes())
        self.assertIn('verificationMaterial', bundle)
        v.verify_manifest(FIXTURES, self.policy, self.run_command)
        args = self.calls[-1]
        self.assertIn('--certificate-github-workflow-ref', args)
        self.assertEqual(args[args.index('--certificate-github-workflow-ref') + 1], 'refs/tags/v0.1.1')
        self.assertEqual(args[args.index('--certificate-github-workflow-sha') + 1],
                         'c0ffbdc4d9bc0f9fda8fde51f4e783224af47104')
        self.assertFalse(any(arg.startswith('--insecure') or arg.startswith('--ignore') for arg in args))

    def test_dispatch_nonrelease_wrong_commit_digest_and_repository_rejected(self):
        cases = [dict(ref='refs/heads/main'), dict(ref='refs/tags/test'), dict(commit='wrong'),
                 dict(images={}), dict(images=dict(self.manifest['images'], server='ghcr.io/evil/server@sha256:'+'a'*64)),
                 dict(images=dict(self.manifest['images'], server='ghcr.io/dannyota/trachuan-server:latest')),
                 dict(images=dict(self.manifest['images'], server='ghcr.io/dannyota/trachuan-server@sha256:')),
                 dict(images=dict(self.manifest['images'], other='ghcr.io/other@sha256:'+'a'*64))]
        for changes in cases:
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                v.release(canonical(dict(self.manifest, **changes)), self.policy)
        with self.assertRaises(ValueError):
            v.release((FIXTURES / 'manifest.json').read_bytes() + b'\n', self.policy)

    def test_signature_rejection_propagates_for_unprotected_identity_and_wrong_sha(self):
        for reason in ('unprotected release workflow', 'dispatch certificate identity', 'wrong certificate SHA'):
            def reject(args, **kwargs):
                raise CommandError(reason)
            with self.subTest(reason=reason), self.assertRaises(CommandError):
                v.verify_manifest(FIXTURES, self.policy, reject)

    def test_producer_gate_rejects_unprotected_and_nonrelease_refs(self):
        workflow = (Path(__file__).parents[3] / '.github/workflows/images.yml').read_text()
        source = workflow.split('      - name: Require protected release from main\n', 1)[1]
        source = source.split('        run: |\n', 1)[1].split('      - name:', 1)[0]
        source = '\n'.join(line[10:] for line in source.splitlines())
        executable = self.path / 'git'
        executable.write_text('#!/bin/bash\nexit 0\n')
        executable.chmod(0o755)
        for ref, protected, accepted in [('refs/tags/v0.1.1', 'false', False),
                                         ('refs/heads/main', 'true', False),
                                         ('refs/tags/v0.1.1', 'true', True)]:
            env = dict(os.environ, GITHUB_REF=ref, REF_PROTECTED=protected, GITHUB_SHA='a' * 40,
                       RUNNER_TEMP=str(self.path), GITHUB_ENV=str(self.path / 'env'),
                       PATH=str(self.path) + os.pathsep + os.environ['PATH'])
            result = subprocess.run(['bash', '-euo', 'pipefail', '-c', source], env=env, capture_output=True)
            self.assertEqual(result.returncode == 0, accepted)

    def test_attestation_requires_subject_ref_repository_commit_and_workflow_path(self):
        image = self.manifest['images']['server']
        good = statement(self.manifest, image)
        v.provenance(good, self.manifest, image, self.policy)
        paths = [('subject', 0, 'digest', 'sha256'), ('subject', 0, 'name'),
                 ('predicate', 'buildDefinition', 'externalParameters', 'workflow', 'ref'),
                 ('predicate', 'buildDefinition', 'externalParameters', 'workflow', 'repository'),
                 ('predicate', 'buildDefinition', 'externalParameters', 'workflow', 'path'),
                 ('predicate', 'buildDefinition', 'resolvedDependencies', 0, 'digest', 'gitCommit'),
                 ('predicate', 'buildDefinition', 'resolvedDependencies', 0, 'uri')]
        for path in paths:
            for missing in (False, True):
                bad = copy.deepcopy(good)
                parent = bad
                for key in path[:-1]:
                    parent = parent[key]
                if missing:
                    del parent[path[-1]]
                else:
                    parent[path[-1]] = 'wrong'
                with self.subTest(path=path, missing=missing), self.assertRaises(ValueError):
                    v.provenance(bad, self.manifest, image, self.policy)

    def test_images_use_digest_and_new_bundle_attestations_with_no_gh_fallback(self):
        present = set()
        def uncached(args, **kwargs):
            if ('inspect' in args or 'exists' in args) and args[-1] not in present:
                self.calls.append(args)
                raise CommandError('image absent', returncode=1)
            if 'pull' in args:
                present.add(args[-1])
            return self.run_command(args, **kwargs)
        verifier = v.Verifier(self.policy, uncached, user_prefix=['as-user'])
        verifier.images(self.manifest)
        pulls = [a for a in self.calls if 'pull' in a]
        self.assertEqual(len(pulls), 5)
        self.assertTrue(all(a[:3] == ['as-user', 'podman', 'pull'] and '@sha256:' in a[-1] for a in pulls))
        attestations = [a for a in self.calls if 'verify-attestation' in a]
        self.assertEqual(len(attestations), 4)
        self.assertTrue(all('--new-bundle-format' in a and 'slsaprovenance1' in a for a in attestations))
        self.assertFalse(any('gh' in a for a in self.calls))

    def test_cached_images_still_require_build_attestations(self):
        v.Verifier(self.policy, self.run_command, user_prefix=['as-user']).images(self.manifest)
        self.assertFalse(any('pull' in args for args in self.calls))
        self.assertEqual(len([args for args in self.calls if 'inspect' in args]), 5)
        self.assertEqual(len([args for args in self.calls if 'verify-attestation' in args]), 4)

    def test_wrong_local_digest_stops_before_attestation(self):
        def wrong(args, **kwargs):
            if 'inspect' in args:
                return b'[{"RepoDigests": []}]'
            return self.run_command(args, **kwargs)
        with self.assertRaisesRegex(ValueError, 'local digest'):
            v.Verifier(self.policy, wrong, user_prefix=[]).images(self.manifest)
        self.assertFalse(any('verify-attestation' in a for a in self.calls))

    def test_bundle_hash_size_placeholders_and_unsafe_paths_rejected(self):
        data = archive()
        manifest = dict(self.manifest, host_bundle={'sha256': hashlib.sha256(data).hexdigest(), 'size': len(data)})
        path = self.path / 'host-bundle.tar.gz'
        path.write_bytes(data)
        files = v.bundle(path, manifest)
        self.assertEqual(files['server.env'], b'GOOGLE_CLIENT_ID=public\n')
        for bad in (data + b'x', archive(server=b'GOOGLE_CLIENT_ID=REPLACE'), archive(web=b'REPLACE')):
            path.write_bytes(bad)
            changed = dict(manifest, host_bundle={'sha256': hashlib.sha256(bad).hexdigest(), 'size': len(bad)})
            with self.assertRaises(ValueError):
                v.bundle(path, manifest if bad == data + b'x' else changed)
        path.write_bytes(bytes([data[0] ^ 1]) + data[1:])
        with self.assertRaisesRegex(ValueError, 'hash mismatch'):
            v.bundle(path, manifest)
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode='w:gz') as tar:
            entry = tarfile.TarInfo('deploy/host/../../escape')
            tar.addfile(entry, io.BytesIO())
        bad = output.getvalue()
        path.write_bytes(bad)
        manifest['host_bundle'] = {'sha256': hashlib.sha256(bad).hexdigest(), 'size': len(bad)}
        with self.assertRaisesRegex(ValueError, 'unsafe'):
            v.bundle(path, manifest)

    def test_corrupt_tar_with_valid_hash_is_a_verification_rejection(self):
        path = self.path / 'host-bundle.tar.gz'
        data = b'not a gzip archive'
        path.write_bytes(data)
        manifest = dict(self.manifest, host_bundle={'sha256': hashlib.sha256(data).hexdigest(), 'size': len(data)})
        with self.assertRaisesRegex(ValueError, 'invalid bundle archive'):
            v.bundle(path, manifest)


if __name__ == '__main__':
    unittest.main()
