from pathlib import Path
import resource
import signal
import subprocess
import sys

SIGNALS = (signal.SIGHUP, signal.SIGINT, signal.SIGQUIT, signal.SIGTERM)


def terminate(number, frame):
    for item in SIGNALS:
        signal.signal(item, signal.SIG_IGN)
    raise SystemExit(128 + number)


def fail(message):
    print(message, file=sys.stderr, flush=True)
    raise SystemExit(1)


def guarded(function):
    resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
    for item in SIGNALS:
        signal.signal(item, terminate)
    try:
        function()
    except subprocess.CalledProcessError as error:
        fail(f'command failed: {Path(str(error.cmd[0])).name} exit {error.returncode}')
    except ValueError as error:
        fail(str(error) if type(error) is ValueError else type(error).__name__)
    except Exception as error:
        fail(type(error).__name__)
