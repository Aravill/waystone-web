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
	folders := map[string]string{} // "parent/name" -> folder id
	seedFolder := func(worldID, parent, name, userID string) (string, error) {
		key := parent + "/" + name
		if id, ok := folders[key]; ok {
			return id, nil
		}
		f := models.Folder{ID: GenerateUUID(), WorldID: worldID, ParentID: parent, Name: name, CreatedBy: userID}
		if err := store.CreateFolder(f); err != nil {
			return "", fmt.Errorf("failed to seed folder %q: %w", name, err)
		}
		folders[key] = f.ID
		return f.ID, nil
	}
	create := func(sp config.CodexSeedPage) (string, error) {
		p := models.Page{ID: GenerateUUID(), Kind: models.PageLore, Title: sp.Title, Aliases: sp.Aliases, AuthorID: sp.AuthorID}
		if sp.InWorld {
			p.WorldID = w.ID
			parent := ""
			for _, name := range sp.Folder {
				id, err := seedFolder(w.ID, parent, name, sp.AuthorID)
				if err != nil {
					return "", err
				}
				parent = id
			}
			p.FolderID = parent
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
	// A page with a map and pins. Pin targets are pages of the world, so these run after all pages exist.
	seedMap := func(pageID, authorID string, pins []config.CodexSeedPin) error {
		file := GenerateUUID() + ".png"
		img := demoMapPNG()
		if err := WriteUpload(file, func(w io.Writer) error { return png.Encode(w, img) }); err != nil {
			return fmt.Errorf("failed to seed map image: %w", err)
		}
		b := img.Bounds()
		if err := store.AddMapImage(models.MapImage{ID: GenerateUUID(), PageID: pageID, File: file, Width: b.Dx(), Height: b.Dy(),
			UploadedBy: authorID, CreatedAt: now}); err != nil {
			if dir, derr := UploadDir(); derr == nil {
				os.Remove(filepath.Join(dir, file))
			}
			return fmt.Errorf("failed to seed map image: %w", err)
		}
		for _, sp := range pins {
			if _, err := store.AddMapPin(models.MapPin{MapPageID: pageID, X: sp.X, Y: sp.Y, TargetText: sp.Target, CreatedBy: authorID}); err != nil {
				return fmt.Errorf("failed to seed map pin: %w", err)
			}
		}
		return nil
	}
	ids := map[string]string{}
	for _, sp := range config.InitialCodexPages {
		id, err := create(sp)
		if err != nil {
			return err
		}
		ids[sp.Title] = id
	}
	mapID, err := create(config.InitialMapPage)
	if err != nil {
		return err
	}
	if err := seedMap(mapID, config.InitialMapPage.AuthorID, config.InitialMapPins); err != nil {
		return err
	}
	if id, ok := ids[config.InitialSettlementMapPage]; ok {
		return seedMap(id, seedAuthor(config.InitialCodexPages, config.InitialSettlementMapPage), config.InitialSettlementMapPins)
	}
	return nil
}

func seedAuthor(pages []config.CodexSeedPage, title string) string {
	for _, p := range pages {
		if p.Title == title {
			return p.AuthorID
		}
	}
	return ""
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
