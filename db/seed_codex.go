package db

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"time"
	"waystone-web/config"
	"waystone-web/models"
)

// seedCodex adds the demo world, its pages, a map with pins and one Unsorted page. Pages are
// created first and linked by title, so the missing link ([[The Ember Choir]]) and the missing
// pin ("Hollow Tor") stay missing on purpose. Runs only on a fresh demo database.
func seedCodex() error {
	w := config.InitialWorld
	now := time.Now().UTC().Format(time.RFC3339)
	w.CreatedAt, w.UpdatedAt = now, now
	if err := store.CreateWorld(w); err != nil {
		return fmt.Errorf("failed to seed world: %w", err)
	}
	if err := store.SetCampaignWorld(config.InitialWorldCampaignID, w.ID); err != nil {
		return fmt.Errorf("failed to set the seed game's world: %w", err)
	}
	create := func(sp config.CodexSeedPage) (string, error) {
		p := models.Page{ID: GenerateUUID(), Kind: sp.Kind, Title: sp.Title, Aliases: sp.Aliases, AuthorID: sp.AuthorID}
		if sp.InWorld {
			p.WorldID = w.ID
		}
		if err := store.CreatePage(p, sp.Body); err != nil {
			return "", fmt.Errorf("failed to seed page %q: %w", sp.Title, err)
		}
		for i, a := range sp.Amend {
			if _, err := store.SavePageRevision(p.ID, i+1, models.PageRevision{AuthorID: a.AuthorID, Body: a.Body, Note: a.Note}, PageKeepRevisions); err != nil {
				return "", fmt.Errorf("failed to amend seed page %q: %w", sp.Title, err)
			}
		}
		return p.ID, nil
	}
	for _, sp := range config.InitialCodexPages {
		if _, err := create(sp); err != nil {
			return err
		}
	}
	mapID, err := create(config.InitialMapPage)
	if err != nil {
		return err
	}

	file := GenerateUUID() + ".png"
	img := demoMapPNG()
	if err := WriteUpload(file, func(w io.Writer) error { return png.Encode(w, img) }); err != nil {
		return fmt.Errorf("failed to seed map image: %w", err)
	}
	b := img.Bounds()
	if err := store.AddMapImage(models.MapImage{ID: GenerateUUID(), PageID: mapID, File: file, Width: b.Dx(), Height: b.Dy(),
		UploadedBy: config.InitialMapPage.AuthorID, CreatedAt: now}); err != nil {
		if dir, derr := UploadDir(); derr == nil {
			os.Remove(filepath.Join(dir, file))
		}
		return fmt.Errorf("failed to seed map image: %w", err)
	}
	for _, sp := range config.InitialMapPins {
		if _, err := store.AddMapPin(models.MapPin{MapPageID: mapID, X: sp.X, Y: sp.Y, TargetText: sp.Target, CreatedBy: config.InitialMapPage.AuthorID}); err != nil {
			return fmt.Errorf("failed to seed map pin: %w", err)
		}
	}
	return nil
}

// demoMapPNG draws a small vellum map with a road, a river and a few landmarks. Nothing is
// checked in as a binary; the image is generated into the upload directory when seeding.
func demoMapPNG() *image.Paletted {
	const w, h = 640, 400
	pal := color.Palette{
		color.RGBA{0xE9, 0xDD, 0xC0, 0xFF}, // vellum
		color.RGBA{0x2B, 0x24, 0x1C, 0xFF}, // ink
		color.RGBA{0x7C, 0x9A, 0xA0, 0xFF}, // river
		color.RGBA{0xB9, 0xA8, 0x85, 0xFF}, // hills
		color.RGBA{0x8E, 0x2A, 0x1E, 0xFF}, // rubric
	}
	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	dot := func(cx, cy, r int, c uint8) {
		for y := cy - r; y <= cy+r; y++ {
			for x := cx - r; x <= cx+r; x++ {
				if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r && x >= 0 && y >= 0 && x < w && y < h {
					img.SetColorIndex(x, y, c)
				}
			}
		}
	}
	line := func(x0, y0, x1, y1, r int, c uint8) {
		steps := 400
		for i := 0; i <= steps; i++ {
			dot(x0+(x1-x0)*i/steps, y0+(y1-y0)*i/steps, r, c)
		}
	}
	// Hills in the north-east.
	for i := 0; i < 9; i++ {
		dot(380+i*28, 70+(i%3)*22, 16, 3)
	}
	// River along the south-west, then the road climbing north-east.
	line(0, 250, 120, 300, 5, 2)
	line(120, 300, 220, 330, 5, 2)
	line(220, 330, 300, 400, 5, 2)
	line(77, 312, 270, 220, 2, 1)
	line(270, 220, 410, 136, 2, 1)
	line(410, 136, 538, 72, 2, 1)
	// Landmarks along the road.
	for _, p := range [][2]int{{77, 312}, {270, 220}, {410, 136}, {538, 72}} {
		dot(p[0], p[1], 6, 1)
		dot(p[0], p[1], 3, 4)
	}
	return img
}
