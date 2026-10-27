"""Exactly one stateful short command. argv paths are substituted by the host adapter."""
import hashlib
import json
from pathlib import Path
import sys

def main():
    w, index, seed = Path(sys.argv[1]), int(sys.argv[2]), int(sys.argv[3])
    previous = 'START' if index == 0 else (w/'steps'/f'{index-1:06d}.txt').read_text()
    digest = hashlib.sha256(f'{previous}:{index}:{seed}'.encode()).hexdigest()
    out = w/'steps'/f'{index:06d}.txt'
    with out.open('x') as f:
        f.write(digest)
    print(json.dumps({'index': index, 'digest': digest}))

if __name__ == '__main__':
    main()
