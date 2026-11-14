from datetime import datetime, timezone

from .deploy_io import Result, atomic, canonical


class ReleaseResult(Result):
    def __init__(self, paths, clock):
        super().__init__(paths.result.with_name('release.json'), paths.log.with_name('release.log'),
                         lambda: datetime.fromtimestamp(clock(), timezone.utc).isoformat().replace('+00:00', 'Z'))

    def write(self, status, step, message, digest=None, manifest=None, mode=None):
        manifest = manifest or {}
        finished = self.clock()
        waiting = self.previous.get('waiting_since')
        waiting_hash = self.previous.get('waiting_manifest_hash', self.previous.get('manifest_hash'))
        if status == 'waiting':
            waiting = waiting if waiting is not None and waiting_hash == digest else finished
            waiting_hash = digest
        elif status in ('ok', 'current', 'verified'):
            waiting, waiting_hash = None, None
        value = dict(status=status, step=step, message=message, manifest_hash=digest,
                     release=manifest.get('rel'), rel=manifest.get('rel'), mode=mode,
                     sequence=manifest.get('sequence'), extensions=manifest.get('extensions'),
                     started_at=self.started, finished_at=finished,
                     waiting_since=waiting, waiting_manifest_hash=waiting_hash if waiting else None,
                     last_success_at=finished if status in ('ok', 'verified', 'current')
                     else self.previous.get('last_success_at'))
        atomic(self.path, canonical(value) + b'\n', 0o640)
        self.append(value)
        self.previous = value
