import unittest
from textutil.slug import slugify

class TestSlug(unittest.TestCase):
    def test_basic(self):
        self.assertEqual(slugify("hello world"), "hello-world")
