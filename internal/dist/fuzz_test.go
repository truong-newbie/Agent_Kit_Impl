package dist

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func FuzzParseRegistryIndex(f *testing.F) {
	f.Add([]byte(`{"schemaVersion":1,"packages":[]}`))
	f.Add([]byte(`{"schemaVersion":1,"packages":[{"id":"eng","version":"0.1.0","url":"a.tar.gz","archiveSha256":"` + strings.Repeat("a", 64) + `"}]}`))
	f.Add([]byte(`{"schemaVersion":1,"schemaVersion":1,"packages":[]}`))
	f.Add([]byte(`not-json`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseRegistryIndex(data, "https://registry.example.com/index.json")
	})
}

func FuzzReadArchiveBytes(f *testing.F) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "package.json", Typeflag: tar.TypeReg, Size: 2, Mode: 0644})
	_, _ = tw.Write([]byte("{}"))
	_ = tw.Close()
	_ = gz.Close()
	f.Add(buf.Bytes())
	f.Add([]byte("not-gzip"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _, _, _, _ = ReadArchiveBytes(raw, "")
	})
}
