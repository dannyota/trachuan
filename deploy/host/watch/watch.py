from pathlib import Path
import sys

sys.dont_write_bytecode = True
directory = Path(__file__).resolve().parent
sys.path.insert(0, str(directory.parent if (directory / '__init__.py').is_file() else directory))

from watch.runtime import main


if __name__ == '__main__':
    sys.exit(main())
