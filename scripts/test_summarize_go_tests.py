import importlib.util
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('timings', Path(__file__).with_name('summarize-go-tests.py'))
timings = importlib.util.module_from_spec(spec)
spec.loader.exec_module(timings)


class TimingTests(unittest.TestCase):
    def test_separate_package_parent_and_child_and_preserve_failures(self):
        events = [
            dict(Action='output', Output='arbitrary test output'),
            dict(Action='pass', Package='p', Elapsed=9),
            dict(Action='pass', Package='p', Test='TestParent', Elapsed=8),
            dict(Action='fail', Package='p', Test='TestParent/a|b', Elapsed=7),
            dict(Action='skip', Package='p', Test='TestSkipped', Elapsed=0),
        ]
        text = timings.summarize(map(json.dumps, events))
        self.assertIn('| 9.000 | p | pass |', text)
        self.assertIn('| 8.000 | p · TestParent | pass |', text)
        self.assertIn('| 7.000 | p · TestParent/a\\|b | fail |', text)
        self.assertNotIn('TestSkipped', text)
        self.assertLess(text.index('### Top-level tests'), text.index('TestParent'))
        self.assertLess(text.index('### Subtests'), text.index('TestParent/a'))


if __name__ == '__main__':
    unittest.main()
