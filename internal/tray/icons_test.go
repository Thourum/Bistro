package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestEmbeddedIconVariants(t *testing.T) {
	const (
		iconSize     = 44
		badgeCenter  = 38
		primaryHex   = 0x004ac6
		errorHex     = 0xba1a1a
		channelBits  = 8
		opaqueAlpha  = 255
		alphaDivisor = 2
	)
	icons := make(map[Icon]image.Image)
	for _, name := range []Icon{IconIdle, IconSyncing, IconAttention, IconSignedOut} {
		encoded, err := iconFiles.ReadFile("icons/" + string(name) + ".png")
		if err != nil {
			t.Fatal(err)
		}
		icon, err := png.Decode(bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		if icon.Bounds() != image.Rect(0, 0, iconSize, iconSize) {
			t.Fatalf("%s dimensions: %v", name, icon.Bounds())
		}
		icons[name] = icon
	}
	for name, hex := range map[Icon]uint32{IconSyncing: primaryHex, IconAttention: errorHex} {
		pixel := color.NRGBAModel.Convert(icons[name].At(badgeCenter, badgeCenter)).(color.NRGBA)
		if pixel != (color.NRGBA{R: uint8(hex >> (channelBits * 2)), G: uint8(hex >> channelBits), B: uint8(hex), A: opaqueAlpha}) {
			t.Fatalf("%s badge color: %v", name, pixel)
		}
	}
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			idle := color.NRGBAModel.Convert(icons[IconIdle].At(x, y)).(color.NRGBA)
			faded := color.NRGBAModel.Convert(icons[IconSignedOut].At(x, y)).(color.NRGBA)
			if faded.R != faded.G || faded.G != faded.B || faded.A != idle.A/alphaDivisor {
				t.Fatalf("signed-out pixel (%d,%d) is not greyscale at half alpha: %v vs %v", x, y, faded, idle)
			}
		}
	}
}
