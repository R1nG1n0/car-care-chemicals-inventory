package barcode

import "image"

func longSide(img *image.Gray) int {
	b := img.Bounds()
	return max(b.Dx(), b.Dy())
}

// toGray переводит изображение в оттенки серого с началом координат (0,0).
func toGray(src image.Image) *image.Gray {
	b := src.Bounds()
	g := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, gr, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			// Яркость по Rec. 601, 16 бит → 8.
			l := (299*r + 587*gr + 114*bl) / 1000
			g.Pix[y*g.Stride+x] = uint8(l >> 8)
		}
	}
	return g
}

// shrink уменьшает изображение усреднением по площади так, чтобы длинная
// сторона стала не больше side.
func shrink(src *image.Gray, side int) *image.Gray {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w, h
	if w >= h {
		nw, nh = side, max(1, h*side/w)
	} else {
		nw, nh = max(1, w*side/h), side
	}
	dst := image.NewGray(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := y*h/nh, max(y*h/nh+1, (y+1)*h/nh)
		for x := 0; x < nw; x++ {
			x0, x1 := x*w/nw, max(x*w/nw+1, (x+1)*w/nw)
			var sum, n int
			for yy := y0; yy < y1; yy++ {
				row := src.Pix[yy*src.Stride:]
				for xx := x0; xx < x1; xx++ {
					sum += int(row[xx])
					n++
				}
			}
			dst.Pix[y*dst.Stride+x] = uint8(sum / n)
		}
	}
	return dst
}

// rotate90 поворачивает изображение на 90° по часовой стрелке.
func rotate90(src *image.Gray) *image.Gray {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewGray(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Pix[x*dst.Stride+(h-1-y)] = src.Pix[y*src.Stride+x]
		}
	}
	return dst
}
