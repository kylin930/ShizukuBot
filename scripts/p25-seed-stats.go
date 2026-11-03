//go:build ignore

// Test-only seed for the isolated P2.5 browser fixture. Never connects to QQ.
// Usage: go run scripts/p25-seed-stats.go /absolute/temporary-fixture-directory
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	stats "shizukubot/storage/stats"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		panic("temporary fixture directory is required")
	}
	root := os.Args[1]
	if !filepath.IsAbs(root) {
		panic("absolute fixture directory required")
	}
	s, e := stats.Open(filepath.Join(root, "data/stats.db"), stats.Defaults(), nil)
	if e != nil {
		panic(e)
	}
	for _, delta := range []int{0, 1, 6, 10, 32} {
		at := time.Now().AddDate(0, 0, -delta)
		for i := 0; i < 2; i++ {
			s.Record(stats.Event{Bot: "alpha", Kind: "received", At: at})
		}
		s.Record(stats.Event{Bot: "alpha", Kind: "sent", At: at})
		s.Record(stats.Event{Bot: "alpha", Kind: "user", Identity: "fixture-user", At: at})
		s.Record(stats.Event{Bot: "alpha", Kind: "group", Identity: "fixture-group", At: at})
	}
	s.Record(stats.Event{Bot: "beta", Kind: "user", Identity: "fixture-user"})
	if e = s.Close(); e != nil {
		panic(e)
	}
	img := image.NewRGBA(image.Rect(0, 0, 256, 144))
	for y := 0; y < 144; y++ {
		for x := 0; x < 256; x++ {
			img.Set(x, y, color.RGBA{uint8(40 + x/3), uint8(80 + y/2), 160, 255})
		}
	}
	file, e := os.Create(filepath.Join(root, "upload-test.png"))
	if e != nil {
		panic(e)
	}
	e = png.Encode(file, img)
	if e != nil {
		panic(e)
	}
	file.Close()
	fmt.Println("Seeded local fixture statistics and upload image")
}
