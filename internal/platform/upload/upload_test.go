package upload

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func picture(t *testing.T, kind string, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{R: 242, A: 255})
		}
	}
	buf := &bytes.Buffer{}
	var err error
	switch kind {
	case "png":
		err = png.Encode(buf, img)
	case "jpg":
		err = jpeg.Encode(buf, img, nil)
	case "gif":
		err = gif.Encode(buf, img, nil)
	default:
		t.Fatalf("unknown fixture kind %q", kind)
	}
	if err != nil {
		t.Fatalf("encode %s: %v", kind, err)
	}
	return buf.Bytes()
}

func TestSaveKeepsRealPictures(t *testing.T) {
	for kind, ext := range map[string]string{"png": ".png", "jpg": ".jpg", "gif": ".gif"} {
		root := t.TempDir()
		result, err := New(root, "products").Save(picture(t, kind, 4, 3))
		if err != nil {
			t.Fatalf("%s upload refused: %v", kind, err)
		}
		if !strings.HasPrefix(result.URL, "/uploads/products/") || !strings.HasSuffix(result.URL, ext) {
			t.Errorf("%s saved to %q", kind, result.URL)
		}
		if result.Width != 4 || result.Height != 3 || result.Size == 0 {
			t.Errorf("%s reported %dx%d in %d bytes", kind, result.Width, result.Height, result.Size)
		}
		name := strings.TrimPrefix(result.URL, "/uploads/products/")
		if _, err := os.Stat(filepath.Join(root, "products", name)); err != nil {
			t.Errorf("%s was not written where the URL says: %v", kind, err)
		}
	}
}

// The name of an upload comes from a client; where it goes comes from this
// program. Two shops uploading at the same second must not collide either.
func TestSaveNamesEveryFileItself(t *testing.T) {
	root := t.TempDir()
	store := New(root, "products")
	first, err := store.Save(append([]byte{}, picture(t, "png", 2, 2)...))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Save(picture(t, "png", 2, 2))
	if err != nil {
		t.Fatal(err)
	}
	if first.URL == second.URL {
		t.Fatalf("one upload replaced the other: %s", first.URL)
	}
}

func TestSaveRefusesWhatIsNotAPicture(t *testing.T) {
	cases := map[string][]byte{
		"empty":           {},
		"an svg drawing":  []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><rect/></svg>`),
		"a script":        []byte("#!/bin/sh\nrm -rf /\n"),
		"a truncated png": []byte("\x89PNG\r\n\x1a\n\x00\x00\x00"),
		"html disguised":  []byte("<html><body>hi</body></html>"),
	}
	for label, data := range cases {
		root := t.TempDir()
		if _, err := New(root, "products").Save(data); !errors.Is(err, ErrNotImage) && !errors.Is(err, ErrEmpty) {
			t.Errorf("%s accepted as %v", label, err)
		}
		entries, _ := os.ReadDir(root)
		for _, entry := range entries {
			t.Errorf("%s left %q behind", label, entry.Name())
		}
	}
}

func TestSaveRefusesAPictureThatIsTooLarge(t *testing.T) {
	data := picture(t, "png", 3, 3)
	data = append(data, bytes.Repeat([]byte{0}, MaxBytes)...)

	root := t.TempDir()
	if _, err := New(root, "products").Save(data); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a %d byte image returned %v", len(data), err)
	}
}

// A hand-built GIF header is 13 bytes and claims 9000×9000: small enough to
// upload, ruinous to decode on every product card.
func TestSaveRefusesAPictureThatClaimsTooManyPixels(t *testing.T) {
	header := []byte("GIF89a")
	header = binary.LittleEndian.AppendUint16(header, 9000)
	header = binary.LittleEndian.AppendUint16(header, 9000)
	header = append(header, 0, 0, 0)

	root := t.TempDir()
	if _, err := New(root, "products").Save(header); !errors.Is(err, ErrHuge) {
		t.Errorf("9000×9000 returned %v", err)
	}
}

// The directory a store writes to is fixed when it is built, so an upload can
// never be aimed at the rest of the filesystem.
func TestStoreWritesOnlyInsideItsOwnDirectory(t *testing.T) {
	root := t.TempDir()
	if _, err := New(root, "site").Save(picture(t, "png", 2, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "site")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
}
