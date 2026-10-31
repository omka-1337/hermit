#!/usr/bin/env python3
"""Generate THIRD-PARTY.md from the dependencies actually shipped with Hermit.

Run from the repository root (or via `task third-party`):

    python3 build/licenses/third-party.py

Go modules come from the build graph of the main package, npm packages from the
production dependency tree, so build-time tools are left out. Licenses are read
from each package's own metadata; anything the script cannot identify is listed
as UNKNOWN so it gets a human look instead of being silently dropped.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "THIRD-PARTY.md"

# Bundled system libraries. They are not dependencies of the source tree: the
# AppImage picks them up from the build host, so they are listed by family.
SYSTEM_LIBRARIES = [
    ("GTK 4", "LGPL-2.1-or-later", "https://gitlab.gnome.org/GNOME/gtk"),
    ("WebKitGTK, JavaScriptCore", "LGPL-2.1-or-later and BSD-2-Clause", "https://webkit.org"),
    ("GLib, GObject, GIO", "LGPL-2.1-or-later", "https://gitlab.gnome.org/GNOME/glib"),
    ("libsoup", "LGPL-2.0-or-later", "https://gitlab.gnome.org/GNOME/libsoup"),
    ("libmanette", "LGPL-2.1-or-later", "https://gitlab.gnome.org/GNOME/libmanette"),
    ("Cairo, Pango, HarfBuzz, Fontconfig, FreeType", "LGPL-2.1-or-later, MPL-1.1, MIT or FTL", "https://www.freedesktop.org"),
    ("GStreamer", "LGPL-2.1-or-later", "https://gstreamer.freedesktop.org"),
    ("ICU", "Unicode-3.0", "https://icu.unicode.org"),
    ("zlib, libpng, libjpeg-turbo, libwebp", "Zlib, libpng-2.0, IJG or BSD-3-Clause", "https://www.freedesktop.org"),
    ("AppImage runtime", "MIT", "https://github.com/AppImage/type2-runtime"),
]

LICENSE_FILES = re.compile(r"^(LICEN[CS]E|COPYING|NOTICE)", re.IGNORECASE)


def identify(text: str) -> str:
    """Name the license of a license file, as an SPDX identifier where possible."""
    t = " ".join(text.split())
    # Before the GPLs: MPL 2.0 names them as its secondary licenses. It is
    # spelled "Version 2.0" or "version 2.0", with or without a comma.
    if re.search(r"Mozilla Public License,? version 2\.0", t, re.IGNORECASE):
        return "MPL-2.0"
    if "GNU AFFERO GENERAL PUBLIC LICENSE" in t.upper():
        return "AGPL-3.0"
    if "GNU LESSER GENERAL PUBLIC LICENSE" in t.upper():
        return "LGPL"
    if "GNU GENERAL PUBLIC LICENSE" in t.upper():
        return "GPL"
    if "covered by two different licenses: MIT and Apache" in t:
        return "MIT and Apache-2.0"
    if "Apache License" in t and "Version 2.0" in t:
        return "Apache-2.0"
    if "Permission is hereby granted, free of charge" in t:
        return "MIT"
    if "Redistribution and use in source and binary forms" in t:
        if "Neither the name" in t or "names of its contributors" in t:
            return "BSD-3-Clause"
        return "BSD-2-Clause"
    if "Permission to use, copy, modify, and/or distribute" in t:
        return "ISC"
    return "UNKNOWN"


def go_modules() -> list[tuple[str, str, str]]:
    """Modules linked into the binary, as (path, version, license)."""
    out = subprocess.run(
        ["go", "list", "-deps", "-f", "{{if .Module}}{{.Module.Path}}\t{{.Module.Version}}\t{{.Module.Dir}}{{end}}", "."],
        cwd=ROOT, capture_output=True, text=True, check=True,
    ).stdout
    modules = {}
    for line in out.splitlines():
        if not line.strip():
            continue
        path, version, directory = line.split("\t")
        if not version:  # the main module
            continue
        modules[path] = (version, directory)

    rows = []
    for path, (version, directory) in sorted(modules.items()):
        license_name = "UNKNOWN"
        d = Path(directory)
        for name in sorted(os.listdir(d)) if d.is_dir() else []:
            if LICENSE_FILES.match(name):
                license_name = identify((d / name).read_text(errors="replace"))
                break
        rows.append((path, version, license_name))
    return rows


def npm_packages() -> list[tuple[str, str, str]]:
    """Packages in the production tree, as (name, version, license)."""
    frontend = ROOT / "frontend"
    out = subprocess.run(
        ["npm", "ls", "--omit=dev", "--all", "--json"],
        cwd=frontend, capture_output=True, text=True,
    ).stdout
    tree = json.loads(out)

    found: dict[tuple[str, str], str] = {}

    def visit(node: dict) -> None:
        for name, info in (node.get("dependencies") or {}).items():
            version = info.get("version", "")
            if (name, version) not in found:
                found[(name, version)] = license_of(frontend, name, info.get("path"))
            visit(info)

    visit(tree)
    return [(name, version, lic) for (name, version), lic in sorted(found.items())]


def license_of(frontend: Path, name: str, path: str | None) -> str:
    """Read a package's declared license, falling back to its license file."""
    directory = Path(path) if path else frontend / "node_modules" / name
    manifest = directory / "package.json"
    if manifest.is_file():
        data = json.loads(manifest.read_text(errors="replace"))
        value = data.get("license") or data.get("licenses")
        if isinstance(value, str):
            return value
        if isinstance(value, dict):
            return value.get("type", "UNKNOWN")
        if isinstance(value, list):
            return " or ".join(v.get("type", "UNKNOWN") for v in value)
    for candidate in sorted(os.listdir(directory)) if directory.is_dir() else []:
        if LICENSE_FILES.match(candidate):
            return identify((directory / candidate).read_text(errors="replace"))
    return "UNKNOWN"


def table(header: tuple[str, str, str], rows: list[tuple[str, str, str]]) -> str:
    lines = [f"| {header[0]} | {header[1]} | {header[2]} |", "| --- | --- | --- |"]
    lines += [f"| `{a}` | {b} | {c} |" for a, b, c in rows]
    return "\n".join(lines)


def main() -> int:
    go_rows = go_modules()
    npm_rows = npm_packages()
    unknown = [r for r in go_rows + npm_rows if r[2] == "UNKNOWN"]

    text = f"""# Third-party licenses

Hermit itself is licensed under the GNU GPL v3 or later (see [LICENSE](LICENSE)). It is built on the work below,
which stays under the licenses listed here.

This file is generated by `build/licenses/third-party.py`; regenerate it after changing dependencies.

## Go modules

Linked into the `hermit` binary.

{table(("Module", "Version", "License"), go_rows)}

## npm packages

Bundled into the frontend. Build-time tools (Vite, TypeScript, Tailwind and their dependencies) are not shipped and
are not listed.

{table(("Package", "Version", "License"), npm_rows)}

## System libraries

The AppImage bundles the GTK and WebKit stack from the build host so Hermit runs on distributions without them.
These libraries keep their own licenses and are shipped as separate shared objects inside the image, so they can be
replaced or relinked; their sources are available from the projects below and from the Ubuntu 24.04 archive the
release is built on.

| Component | License | Source |
| --- | --- | --- |
"""
    text += "\n".join(f"| {name} | {lic} | {url} |" for name, lic, url in SYSTEM_LIBRARIES)
    text += "\n"

    OUTPUT.write_text(text)
    print(f"{OUTPUT.relative_to(ROOT)}: {len(go_rows)} Go modules, {len(npm_rows)} npm packages")
    if unknown:
        print("unidentified licenses:", ", ".join(f"{name}@{version}" for name, version, _ in unknown), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
