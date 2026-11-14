import argparse
import json
import os
from pathlib import Path
import signal
import stat
import sys

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from updater.deploy_run import Updater
from updater.deploy_store import Paths
from updater.deploy_io import Busy, Lock, Result


def interrupted(signum, frame):
    raise InterruptedError('updater interrupted')


def trusted(path, mode):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) != mode:
        raise ValueError('unsafe updater configuration permissions')


def main():
    if os.geteuid() != 0:
        sys.exit('deploy: run as root')
    os.umask(0o077)
    parser = argparse.ArgumentParser()
    subcommands = parser.add_subparsers(dest='command', required=True)
    subcommands.add_parser('run')
    subcommands.add_parser('status')
    subcommands.add_parser('bootstrap').add_argument('manifest_sha256')
    args = parser.parse_args()
    paths = Paths()
    try:
        trusted(paths.policy, 0o644)
        if args.command != 'bootstrap':
            trusted(paths.mode, 0o644)
        app = Updater(paths)
        if args.command == 'status':
            print(json.dumps(app.status(), sort_keys=True))
            return 0
        if args.command == 'bootstrap':
            return app.bootstrap(args.manifest_sha256)
        return app.run()
    except (OSError, ValueError, KeyError, TypeError):
        if args.command != 'status':
            try:
                with Lock(paths.lock), Lock(paths.backup_lock, create=False):
                    Result(paths.result, paths.log).write('failed', 'configuration',
                        'configuration or state invalid; bootstrap may be required')
            except (OSError, ValueError, Busy):
                pass
        print('deploy: configuration or state invalid; bootstrap may be required', file=sys.stderr)
        return 1


if __name__ == '__main__':
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    sys.exit(main())
