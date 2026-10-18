package bookmeta

import (
	"strings"
	"testing"
)

func TestSetEPUBMetadataPreservesAttributes(t *testing.T) {
	opf := []byte(`<?xml version="1.0"?>
<package xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
  <metadata>
    <dc:title id="title">Old Title</dc:title>
    <dc:creator opf:role="aut">Old Author</dc:creator>
  </metadata>
</package>`)

	out, err := setEPUBMetadata(opf, `New & Title`, "New Author")
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{
		`<dc:title id="title">New &amp; Title</dc:title>`,
		`<dc:creator opf:role="aut">New Author</dc:creator>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestSetEPUBMetadataInsertsMissingElement(t *testing.T) {
	opf := []byte(`<package xmlns:dc="http://purl.org/dc/elements/1.1/"><metadata></metadata></package>`)

	out, err := setEPUBMetadata(opf, "Title", "Author")
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "<dc:title>Title</dc:title>") ||
		!strings.Contains(got, "<dc:creator>Author</dc:creator>") {
		t.Fatalf("metadata not inserted:\n%s", got)
	}
}
