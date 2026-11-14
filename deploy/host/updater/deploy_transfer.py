import os
from pathlib import Path
import sys
import tempfile


def program():
    return Path(__file__).read_text()


def main():
    path = Path(sys.argv[1])
    mode = int(sys.argv[2], 8)
    descriptor, name = tempfile.mkstemp(prefix='.deploy-', dir=path.parent)
    try:
        with os.fdopen(descriptor, 'wb') as output:
            output.write(sys.stdin.buffer.read())
            output.flush()
            os.fchmod(output.fileno(), mode)
            os.fsync(output.fileno())
        os.replace(name, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        Path(name).unlink(missing_ok=True)


if __name__ == '__main__':
    main()
