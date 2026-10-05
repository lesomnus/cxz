#!/usr/bin/env python3
"""Summarize Go test JSON without confusing cumulative subtests with wall time."""
import json
from pathlib import Path
import sys


def cell(value):
    return str(value).replace('|', '\\|').replace('\n', ' ').replace('\r', ' ').replace('`', "'")


def summarize(lines):
    groups = {'Packages': [], 'Top-level tests': [], 'Subtests': []}
    for line in lines:
        event = json.loads(line)
        if event.get('Action') not in ('pass', 'fail') or 'Elapsed' not in event:
            continue
        package = event.get('Package', '')
        test = event.get('Test')
        group = 'Packages' if not test else ('Subtests' if '/' in test else 'Top-level tests')
        groups[group].append((event['Elapsed'], package, test, event['Action']))
    out = ['## Go race test timings', '',
           'Durations overlap across packages. Parent tests include subtests; do not sum rows.', '']
    for title, rows in groups.items():
        out += [f'### {title}', '', '| Seconds | Package / test | Result |', '|---:|---|---|']
        for seconds, package, test, result in sorted(rows, key=lambda row: row[0], reverse=True)[:20]:
            label = package + (f' · {test}' if test else '')
            out.append(f'| {seconds:.3f} | {cell(label)} | {result} |')
        out.append('')
    return '\n'.join(out)


def main():
    path = Path(sys.argv[1])
    if not path.exists():
        print('No test timing log was produced; check job setup or cancellation.')
        return
    with path.open() as source:
        print(summarize(source))


if __name__ == '__main__':
    main()
