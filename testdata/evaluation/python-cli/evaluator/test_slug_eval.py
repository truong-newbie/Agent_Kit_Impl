import unittest
from textutil.slug import slugify

class TestSlugEval(unittest.TestCase):
    def test_strips_and_collapses(self):
        self.assertEqual(slugify("  Hello,   World!! "), "hello-world")
        self.assertEqual(slugify("---"), "")
