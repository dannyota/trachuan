from dataclasses import dataclass
import json
from pathlib import Path
import shutil

from .deploy_bundle import bundle
from .deploy_host import pins
from .deploy_io import atomic
from .deploy_verify import verify_manifest
from .deploy_state import digest


class Paths:
    def __init__(self, root=Path('/')):
        self.root = Path(root)
        self.state = self.root / 'var/lib/trachuan-deploy'
        self.host = self.root / 'srv/trachuan/host'
        self.bundles = self.root / 'srv/trachuan/bundles'
        self.quadlet = self.root / 'home/trachuan/.config/containers/systemd'
        self.user_units = self.root / 'home/trachuan/.config/systemd/user'
        self.postgres_unit = self.quadlet / 'trachuan-postgres.container'
        self.result = self.root / 'srv/trachuan/state/deploy/result.json'
        self.hold = self.root / 'srv/trachuan/state/deploy-hold'
        self.log = self.root / 'var/log/trachuan/deploy.log'
        self.lock = self.root / 'run/trachuan-deploy.lock'
        self.backup_lock = self.root / 'srv/trachuan/state/backup/backup.lock'
        self.policy = self.root / 'etc/trachuan/deploy-policy.json'
        self.mode = self.root / 'etc/trachuan/deploy-mode'


@dataclass
class Candidate:
    hash: str
    manifest: dict
    directory: Path
    files: dict

    @classmethod
    def read(cls, directory, rules, runner):
        directory = Path(directory)
        hash_value, manifest = verify_manifest(directory, rules, runner)
        return cls(hash_value, manifest, directory, bundle(directory / 'host-bundle.tar.gz', manifest))


class Store:
    def __init__(self, paths, rules, runner):
        self.paths, self.rules, self.runner = paths, rules, runner
        self.directory = paths.state / 'artifacts'

    def retain(self, candidate):
        directory = self.directory / candidate.hash
        directory.mkdir(parents=True, mode=0o700, exist_ok=True)
        for name in ('manifest.json', 'manifest.sigstore.json', 'host-bundle.tar.gz'):
            atomic(directory / name, (candidate.directory / name).read_bytes())

    def load(self, hash_value):
        if not digest(hash_value):
            raise ValueError('invalid retained hash')
        candidate = Candidate.read(self.directory / hash_value, self.rules, self.runner)
        if candidate.hash != hash_value:
            raise ValueError('retained manifest hash mismatch')
        return candidate

    def unpack(self, candidate):
        directory = self.paths.bundles / candidate.manifest['host_bundle']['sha256']
        directory.mkdir(parents=True, mode=0o755, exist_ok=True)
        for name, data in candidate.files.items():
            if name == 'images.env':
                continue
            path = directory / name
            path.parent.mkdir(parents=True, mode=0o755, exist_ok=True)
            atomic(path, data, 0o755 if name.endswith('.sh') or name == 'updater/trachuan-deploy' else 0o644)
        atomic(directory / 'images.env', pins(candidate.manifest), 0o644)
        return directory

    def prune(self, state):
        keep = {state.data['active'], *state.data['retained']}
        journal = state.data['journal']
        if journal and journal['status'] in ('started', 'recovering'):
            keep.update((journal['target'], journal['previous']))
        bundles = {state.data['installed_bundle']}
        for path in self.directory.iterdir():
            if path.name in keep:
                bundles.add(json.loads((path / 'manifest.json').read_bytes())['host_bundle']['sha256'])
            elif digest(path.name) and path.is_dir() and not path.is_symlink():
                shutil.rmtree(path)
        if self.paths.bundles.exists():
            for path in self.paths.bundles.iterdir():
                if digest(path.name) and path.name not in bundles and path.is_dir() and not path.is_symlink():
                    shutil.rmtree(path)
