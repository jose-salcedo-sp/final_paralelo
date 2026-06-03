package filters

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"github.com/disintegration/imaging"
)

func Apply(filter string, input []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(input))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	var out image.Image
	switch filter {
	case "grayscale":
		out = grayscale(img)
	case "blur":
		out = imaging.Blur(img, 2.2)
	default:
		return nil, fmt.Errorf("unsupported filter: %s", filter)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

func grayscale(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			g := color.GrayModel.Convert(src.At(x, y))
			dst.Set(x, y, g)
		}
	}
	return dst
}
