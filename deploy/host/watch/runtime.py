import logging
from pathlib import Path
import signal
import time

from . import delivery, network, observations, signals, storage


def credentials():
    return network.credentials(storage.read(Path('/etc/trachuan/deploy-policy.json')))


def run(directory=Path('/srv/trachuan/state/watch'),
        logfile=Path('/var/log/trachuan/watch/watch.log'), collect=observations.collect,
        fetch=credentials, send=network.send, clock=time.time):
    now = clock()
    state = storage.load_state(directory / 'state.json')
    def log(value):
        storage.log(logfile, value)
    success = False
    try:
        signals.observe(state, collect(), now)
        success = delivery.deliver(state, now, directory / 'state.json', fetch, send, log, clock=clock)
    except Exception:
        log(dict(timestamp=storage.utc(clock()), message='watch failed'))
    storage.result(directory, success, clock())
    return 0 if success else 1


def interrupted(signum, frame):
    raise InterruptedError('watch interrupted')


def main():
    logging.disable(logging.CRITICAL)
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    try:
        return run()
    except Exception:
        print('watch failed')
        return 1
