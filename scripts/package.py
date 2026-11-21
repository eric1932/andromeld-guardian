#!/usr/bin/env python3
"""Package ONLY the explicit module payload; never include research or device data."""
import hashlib
import json
from pathlib import Path
import re
import zipfile

ROOT = Path(__file__).resolve().parents[1]
MODULE = ROOT / 'module'
PAYLOAD = (
    'bin/guardian', 'customize.sh', 'hotinstall.sh', 'module.prop',
    'service.sh', 'skip_mount', 'uninstall.sh',
    'webroot/app.js', 'webroot/index.html', 'webroot/style.css',
)


def main():
    prop = (MODULE / 'module.prop').read_text()
    version = re.search(r'^version=v(\d+\.\d+\.\d+)$', prop, re.M)
    if version is None:
        raise SystemExit('Invalid module version')
    for name in PAYLOAD:
        path = MODULE / name
        if not path.is_file() or path.is_symlink():
            raise SystemExit('Missing/unsafe explicit payload: ' + name)
    data = (MODULE / 'bin/guardian').read_bytes()
    # ELF64 little-endian, ET_DYN/PIE, AArch64. Do not accidentally package a host binary.
    if (data[:6] != b'\x7fELF\x02\x01' or int.from_bytes(data[16:18], 'little') != 3
            or int.from_bytes(data[18:20], 'little') != 183):
        raise SystemExit('Expected Android arm64 PIE; build first')
    output = ROOT / 'dist'
    output.mkdir(exist_ok=True)
    archive = output / ('AndroMeld-Guardian-v' + version[1] + '-arm64.zip')
    manifest = {}
    with zipfile.ZipFile(archive, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for name in sorted(PAYLOAD):
            content = (MODULE / name).read_bytes()
            item = zipfile.ZipInfo(name, date_time=(2026, 11, 5, 0, 0, 0))
            item.create_system = 3
            mode = 0o100755 if name == 'bin/guardian' or name.endswith('.sh') else 0o100644
            item.external_attr = mode << 16
            item.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(item, content)
            manifest[name] = hashlib.sha256(content).hexdigest()
    with zipfile.ZipFile(archive) as z:
        if z.testzip() is not None or set(z.namelist()) != set(PAYLOAD):
            raise SystemExit('Archive validation failed')
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    (output / 'SHA256SUMS').write_text(digest + '  ' + archive.name + '\n')
    print(archive)
    print(digest, archive.stat().st_size, 'bytes')


if __name__ == '__main__':
    main()
