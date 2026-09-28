// Package upload keeps the pictures a shop adds from the back office. The
// server already serves /uploads/<name> read-only, so the only missing half was
// writing a file there: this package decides what may be kept, what it is
// called, and where it lands.
package upload

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"time"

	// The three formats a browser can be handed without a second thought. A
	// decoder is only registered here so DecodeConfig can read the size of an
	// image without drawing it.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const (
	// MaxBytes is what a shop cover or a logo may weigh. A phone camera photo is
	// routinely 3-8 MB and no storefront needs that; the browser shrinks it first.
	MaxBytes = 2 << 20
	// MaxEdge stops a 40 KB file that claims to be 20000×20000 from turning every
	// product card into a memory problem: the cap is on the picture, not the bytes.
	MaxEdge = 8192
)

var (
	ErrEmpty    = errors.New("没有收到图片文件")
	ErrTooLarge = errors.New("图片超过 2 MB")
	ErrNotImage = errors.New("文件不是可保存的图片")
	ErrHuge     = errors.New("图片像素尺寸过大")
)

// extensions maps what the bytes actually are to the name they are saved under.
// An upload's own filename is never used: it arrives from a client, and the only
// thing it can be trusted for is nothing at all. SVG is absent on purpose — an
// SVG is a script that sometimes draws a picture, and the server would be
// handing it out under the shop's own origin.
var extensions = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
}

// Result describes a saved file well enough for a form to fill itself in.
type Result struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Size   int64  `json:"size"`
}

// Store writes into one fixed subdirectory of the uploads tree. The directory is
// chosen by the code that builds the store, never by the request, so a caller
// cannot aim an upload at a path it has no business touching.
type Store struct {
	root  string // the uploads directory
	scope string // its subdirectory, e.g. "products"
}

func New(root, scope string) *Store {
	return &Store{root: root, scope: scope}
}

// Save verifies the bytes are a picture, names them after the moment they
// arrived, and writes them under the store's own directory.
func (s *Store) Save(data []byte) (Result, error) {
	switch {
	case len(data) == 0:
		return Result{}, ErrEmpty
	case len(data) > MaxBytes:
		return Result{}, ErrTooLarge
	}

	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	ext, ok := extensions[http.DetectContentType(head)]
	if !ok {
		return Result{}, ErrNotImage
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Result{}, ErrNotImage
	}
	if config.Width < 1 || config.Height < 1 || config.Width > MaxEdge || config.Height > MaxEdge {
		return Result{}, ErrHuge
	}

	name, err := uniqueName(ext)
	if err != nil {
		return Result{}, err
	}
	dir := filepath.Join(s.root, s.scope)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, fmt.Errorf("创建图片目录: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return Result{}, fmt.Errorf("写入图片: %w", err)
	}
	return Result{
		URL:    "/uploads/" + s.scope + "/" + name,
		Width:  config.Width,
		Height: config.Height,
		Size:   int64(len(data)),
	}, nil
}

// uniqueName is a timestamp for a human to sort by and random bytes for a human
// not to guess: two shops uploading the same cover on the same second must not
// overwrite each other, and a saved file should not be enumerable.
func uniqueName(ext string) (string, error) {
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("生成图片名: %w", err)
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	return stamp + "-" + hex.EncodeToString(random[:]) + ext, nil
}
