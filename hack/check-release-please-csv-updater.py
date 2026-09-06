#!/usr/bin/env python3
"""Assert release-please updates the OLM bundle CSV generically, not via yaml.

The yaml/jsonpath updater round-trips the whole document through a YAML parse
and re-emit, which drops the inline `# x-release-please-version` markers the
CSV's image references depend on. Release PR #152 is the worked example: the
yaml updater bumped $.spec.version, stripped every marker, and the generic
updater then had nothing to match, so all four image references stayed on the
previous release. See hack/check-chart-image-tags.sh and #129.
"""

import json
import sys


def main(config_path: str, csv_path: str) -> int:
    with open(config_path, encoding="utf-8") as handle:
        config = json.load(handle)

    extra_files = config["packages"]["."]["extra-files"]
    entries = [
        entry
        for entry in extra_files
        if isinstance(entry, dict) and entry.get("path") == csv_path
    ]

    offenders = [entry for entry in entries if entry.get("type") != "generic"]
    if offenders:
        print(f"  FAIL {config_path} points a non-generic updater at {csv_path}:")
        for entry in offenders:
            print(f"       {json.dumps(entry)}")
        print("       That drops the x-release-please-version markers and freezes")
        print("       every image reference. Use the generic updater only.")
        return 1

    if not entries:
        print(f"  FAIL {config_path} has no generic updater for {csv_path};")
        print("       the CSV version and image references would never be bumped.")
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1], sys.argv[2]))
