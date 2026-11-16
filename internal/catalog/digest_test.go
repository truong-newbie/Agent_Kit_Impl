package catalog

import (
	"bytes"
	"testing"
	"testing/fstest"
)

func TestDigestIsStableAndSensitiveToContentAndPath(t *testing.T) {
	a := fstest.MapFS{"a": &fstest.MapFile{Data: []byte("bc")}, "d/f": &fstest.MapFile{Data: []byte("value")}}
	b := fstest.MapFS{"d/f": &fstest.MapFile{Data: []byte("value")}, "a": &fstest.MapFile{Data: []byte("bc")}}
	first, err := Digest(a)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Digest(b)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("digest depends on map insertion order")
	}
	b["a"].Data = []byte("changed")
	third, err := Digest(b)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("digest did not change with content")
	}
	c := fstest.MapFS{"ab": &fstest.MapFile{Data: []byte("c")}, "d/f": &fstest.MapFile{Data: []byte("value")}}
	fourth, err := Digest(c)
	if err != nil {
		t.Fatal(err)
	}
	if first == fourth {
		t.Fatal("ambiguous concatenation in digest")
	}
}

func TestDigestRejectsOversizedFile(t *testing.T) {
	f := fstest.MapFS{"large": &fstest.MapFile{Data: bytes.Repeat([]byte("x"), 1024*1024+1)}}
	if _, err := Digest(f); err == nil {
		t.Fatal("accepted oversized package file")
	}
}
