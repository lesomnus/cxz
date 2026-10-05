import importlib.util
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('publication', Path(__file__).with_name('check-edge-publication.py'))
publication = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publication)


class PublicationTests(unittest.TestCase):
    def check(self, sequence, revision='a', published=10, published_revision='a'):
        with patch.object(publication, 'api', side_effect=[
            {'assets': [{'name': 'cxz-update.json', 'id': 1}]},
            {'sequence': published, 'revision': published_revision},
        ]):
            return publication.should_publish('owner/repo', sequence, revision)

    def test_ordering_and_retry(self):
        self.assertTrue(self.check(11))
        self.assertTrue(self.check(10))
        self.assertFalse(self.check(9))
        with self.assertRaises(ValueError):
            self.check(10, revision='b')

    def test_first_publication(self):
        with patch.object(publication, 'api', return_value=None):
            self.assertTrue(publication.should_publish('owner/repo', 1, 'a'))

    def test_incomplete_release_fails_closed(self):
        with patch.object(publication, 'api', return_value={'assets': []}):
            with self.assertRaises(RuntimeError):
                publication.should_publish('owner/repo', 11, 'a')
        with self.assertRaises(ValueError):
            self.check(11, published='10')

    def test_only_missing_release_is_tolerated(self):
        with patch.object(publication.subprocess, 'run', return_value=subprocess.CompletedProcess([], 1, '', 'gh: Not Found (HTTP 404)')):
            self.assertIsNone(publication.api('path'))
        for error in ['gh: forbidden (HTTP 403)', 'network timeout']:
            with patch.object(publication.subprocess, 'run', return_value=subprocess.CompletedProcess([], 1, '', error)):
                with self.assertRaises(RuntimeError):
                    publication.api('path')


if __name__ == '__main__':
    unittest.main()
