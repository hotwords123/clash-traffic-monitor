package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

func makeIcon(paused bool) []byte {
	canvas := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 12, G: 20, B: 36, A: 255}}, image.Point{}, draw.Src)
	accent := color.RGBA{R: 102, G: 231, B: 199, A: 255}
	if paused {
		accent = color.RGBA{R: 245, G: 174, B: 84, A: 255}
	}
	for y := 4; y < 28; y++ {
		for x := 4; x < 28; x++ {
			dx, dy := x-16, y-16
			if dx*dx+dy*dy <= 135 {
				canvas.Set(x, y, color.RGBA{R: 25, G: 39, B: 65, A: 255})
			}
		}
	}
	if paused {
		draw.Draw(canvas, image.Rect(11, 9, 14, 23), &image.Uniform{C: accent}, image.Point{}, draw.Src)
		draw.Draw(canvas, image.Rect(18, 9, 21, 23), &image.Uniform{C: accent}, image.Point{}, draw.Src)
	} else {
		draw.Draw(canvas, image.Rect(9, 17, 13, 23), &image.Uniform{C: accent}, image.Point{}, draw.Src)
		draw.Draw(canvas, image.Rect(14, 13, 18, 23), &image.Uniform{C: accent}, image.Point{}, draw.Src)
		draw.Draw(canvas, image.Rect(19, 8, 23, 23), &image.Uniform{C: accent}, image.Point{}, draw.Src)
	}
	var output bytes.Buffer
	_ = png.Encode(&output, canvas)
	return output.Bytes()
}
