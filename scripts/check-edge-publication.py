#!/usr/bin/env python3
"""Skip an out-of-order successful CI run before mutating the edge channel."""
import json
import os
import subprocess


def api(path, *args):
    result = subprocess.run(['gh', 'api', path, *args], capture_output=True, text=True)
    if result.returncode:
        # Only a missing release is expected during first publication. Fail closed
        # on authentication, rate limiting, network, or malformed responses.
        if '(HTTP 404)' in result.stderr:
            return None
        raise RuntimeError(result.stderr.strip())
    return json.loads(result.stdout)


def should_publish(repository, sequence, revision):
    release = api(f'repos/{repository}/releases/tags/edge')
    if release is None:
        return True
    asset = next((a for a in release['assets'] if a['name'] == 'cxz-update.json'), None)
    if asset is None:
        raise RuntimeError('Existing edge release has no manifest; refusing an unchecked publication')
    manifest = api(f"repos/{repository}/releases/assets/{asset['id']}",
                   '-H', 'Accept: application/octet-stream')
    if manifest is None:
        raise RuntimeError('Published edge manifest disappeared')
    published_sequence = manifest['sequence']
    if type(published_sequence) is not int or published_sequence <= 0:
        raise ValueError('Invalid published sequence')
    if published_sequence == sequence and manifest['revision'] != revision:
        raise ValueError('Publication sequence belongs to another revision')
    return published_sequence <= sequence


def main():
    publish = should_publish(os.environ['GITHUB_REPOSITORY'],
                             int(os.environ['GITHUB_RUN_NUMBER']), os.environ['GITHUB_SHA'])
    with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
        output.write(f'publish={str(publish).lower()}\n')
    print('Publishing this tested build' if publish else 'A newer build is already published; skipping')


if __name__ == '__main__':
    main()
