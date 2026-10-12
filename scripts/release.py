#!/usr/bin/env python3
"""Build deterministic release archives using Go and the Python standard library."""

import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile
import time
import zipfile

TARGETS = (("linux", "amd64"), ("linux", "arm64"), ("darwin", "amd64"),
           ("darwin", "arm64"), ("windows", "amd64"))
ROOT = Path(__file__).resolve().parents[1]


def run(*args, **kwargs):
    return subprocess.check_output(args, cwd=ROOT, text=True, **kwargs).strip()


def build(version, destination):
    if not re.fullmatch(r"v\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?", version):
        raise ValueError("version must be a v-prefixed semantic version")
    commit = run("git", "rev-parse", "HEAD")
    epoch = int(run("git", "show", "-s", "--format=%ct", "HEAD"))
    module = json.loads(run("go", "list", "-m", "-json", "github.com/glesys/glesys-go/v8"))
    replacement = module.get("Replace", {})
    if not replacement.get("Version"):
        raise ValueError("release requires an exact remote SDK replacement version")
    sdk = module["Version"] + "+fork." + replacement["Version"]
    flags = f"-s -w -buildid= -X main.version={version} -X main.commit={commit} -X main.sdkVersion={sdk}"
    destination.mkdir(parents=True, exist_ok=True)
    hashes = []
    with tempfile.TemporaryDirectory(prefix="elg-release-") as temporary:
        for system, architecture in TARGETS:
            binary_name = "elg.exe" if system == "windows" else "elg"
            binary = Path(temporary) / binary_name
            env = dict(os.environ, GOOS=system, GOARCH=architecture, CGO_ENABLED="0")
            subprocess.run(("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", flags,
                            "-o", str(binary), "./cmd/elg"), cwd=ROOT, env=env, check=True)
            entries = [(binary_name, binary.read_bytes(), 0o755)]
            for name in ("README.md", "LICENSE", "THIRD-PARTY-NOTICES.txt"):
                entries.append((name, (ROOT / name).read_bytes(), 0o644))
            stem = f"elg_{version}_{system}_{architecture}"
            if system == "windows":
                archive = destination / (stem + ".zip")
                with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as zipped:
                    for name, data, mode in entries:
                        info = zipfile.ZipInfo(name, time.gmtime(epoch)[:6])
                        info.compress_type = zipfile.ZIP_DEFLATED
                        info.external_attr = mode << 16
                        zipped.writestr(info, data)
                with zipfile.ZipFile(archive) as zipped:
                    assert set(zipped.namelist()) == {entry[0] for entry in entries}
                    assert zipped.testzip() is None
            else:
                archive = destination / (stem + ".tar.gz")
                with archive.open("wb") as target:
                    with gzip.GzipFile(fileobj=target, mode="wb", filename="", mtime=epoch) as compressed:
                        with tarfile.open(fileobj=compressed, mode="w") as tar:
                            for name, data, mode in entries:
                                info = tarfile.TarInfo(name)
                                info.size, info.mode, info.mtime = len(data), mode, epoch
                                tar.addfile(info, io.BytesIO(data))
                with tarfile.open(archive) as tar:
                    assert set(tar.getnames()) == {entry[0] for entry in entries}
            hashes.append(f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n")
            print(archive.name)
    (destination / "checksums.txt").write_text("".join(hashes), encoding="utf-8")
    assert len(hashes) == len(TARGETS)


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit("usage: release.py VERSION OUTPUT_DIRECTORY")
    build(sys.argv[1], Path(sys.argv[2]).resolve())
