//go:build ignore

// Run from the repository root: go run internal/tray/icons/gen.go.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

const (
	// A 22pt tray icon at 2× resolution.
	iconSize          = 44
	badgeDiameter     = 12
	badgeRadius       = badgeDiameter / 2
	pixelCenterOffset = 0.5
	// Light-theme --color-primary and --color-error in frontend/src/styles/globals.css.
	primaryHex    = 0x004ac6
	errorHex      = 0xba1a1a
	channelMask   = 0xff
	channelBits   = 8
	alphaDivisor  = 2
	iconSource    = "build/appicon.png"
	iconDirectory = "internal/tray/icons"
)

func main() {
	if err := generate(); err != nil {
		log.Fatal(err)
	}
}

func generate() error {
	file, err := os.Open(iconSource)
	if err != nil {
		return err
	}
	original, decodeErr := png.Decode(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return decodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if original.Bounds().Dx() < iconSize || original.Bounds().Dy() < iconSize {
		return fmt.Errorf("source icon must be at least %d×%d", iconSize, iconSize)
	}
	idle := downsample(original)
	variants := map[string]image.Image{
		"idle":       idle,
		"syncing":    addBadge(idle, hexColor(primaryHex)),
		"attention":  addBadge(idle, hexColor(errorHex)),
		"signed_out": fadeGreyscale(idle),
	}
	for name, icon := range variants {
		if err := writePNG(filepath.Join(iconDirectory, name+".png"), icon); err != nil {
			return err
		}
	}
	return nil
}

func downsample(original image.Image) *image.RGBA {
	bounds := original.Bounds()
	icon := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			box := image.Rect(bounds.Min.X+x*bounds.Dx()/iconSize, bounds.Min.Y+y*bounds.Dy()/iconSize,
				bounds.Min.X+(x+1)*bounds.Dx()/iconSize, bounds.Min.Y+(y+1)*bounds.Dy()/iconSize)
			icon.SetRGBA(x, y, averageBox(original, box))
		}
	}
	return icon
}

func averageBox(original image.Image, box image.Rectangle) color.RGBA {
	var red, green, blue, alpha uint64
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			r, g, b, a := original.At(x, y).RGBA()
			red += uint64(r)
			green += uint64(g)
			blue += uint64(b)
			alpha += uint64(a)
		}
	}
	count := uint64(box.Dx() * box.Dy())
	return color.RGBA{R: uint8(red / count >> channelBits), G: uint8(green / count >> channelBits),
		B: uint8(blue / count >> channelBits), A: uint8(alpha / count >> channelBits)}
}

func hexColor(hex uint32) color.RGBA {
	return color.RGBA{R: uint8(hex >> (channelBits * 2)), G: uint8(hex >> channelBits), B: uint8(hex), A: channelMask}
}

func addBadge(original *image.RGBA, badge color.RGBA) *image.RGBA {
	icon := image.NewRGBA(original.Bounds())
	copy(icon.Pix, original.Pix)
	center := float64(iconSize - badgeRadius)
	for y := iconSize - badgeDiameter; y < iconSize; y++ {
		for x := iconSize - badgeDiameter; x < iconSize; x++ {
			// Pixel centers keep the even-diameter circle symmetric.
			dx, dy := float64(x)+pixelCenterOffset-center, float64(y)+pixelCenterOffset-center
			if dx*dx+dy*dy <= badgeRadius*badgeRadius {
				icon.SetRGBA(x, y, badge)
			}
		}
	}
	return icon
}

func fadeGreyscale(original *image.RGBA) *image.NRGBA {
	icon := image.NewNRGBA(original.Bounds())
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			pixel := color.NRGBAModel.Convert(original.At(x, y)).(color.NRGBA)
			grey := color.GrayModel.Convert(color.RGBA{R: pixel.R, G: pixel.G, B: pixel.B, A: channelMask}).(color.Gray).Y
			icon.SetNRGBA(x, y, color.NRGBA{R: grey, G: grey, B: grey, A: pixel.A / alphaDivisor})
		}
	}
	return icon
}

func writePNG(path string, icon image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encodeErr := png.Encode(file, icon)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}
