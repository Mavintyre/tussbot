package main

import (
	"math"
	"strings"

	"github.com/fogleman/gg"
)

func drawCircle() {
	width, height := 100, 100
	cx, cy := 50.0, 50.0
	radius := 30.0
	border := 2.0

	slices := 8.0
	ticked := 3.0

	angle := 360 / (slices / ticked)

	ctx := gg.NewContext(width, height)

	ctx.DrawCircle(cx, cy, radius)
	ctx.SetHexColor("#40444b")
	ctx.Fill()

	ctx.MoveTo(cx, cy)
	ctx.DrawArc(cx, cy, radius, gg.Radians(-90), gg.Radians(angle-90))
	ctx.SetHexColor("#7289da")
	ctx.Fill()

	ctx.SetHexColor("#202225")
	ctx.SetLineWidth(border)
	ctx.DrawCircle(cx, cy, radius)
	for i := 0.0; i < slices; i++ {
		x := cx + radius*math.Cos(2.0*math.Pi*i/slices)
		y := cy + radius*math.Sin(2.0*math.Pi*i/slices)
		ctx.DrawLine(cx, cy, x, y)
	}
	ctx.Stroke()

	ctx.SavePNG("out.png")
}

type Point struct {
	X, Y float64
}

func Spike() []Point {
	poly := make([]Point, 5)
	poly[0] = Point{0, 0}
	poly[1] = Point{-1, 1}
	poly[2] = Point{0, 5}
	poly[3] = Point{1, 1}
	poly[4] = Point{0, 0}
	return poly
}

func drawSpikes() {
	width, height := 300, 100

	slices := 6
	ticked := 2

	cx, cy := 50.0, 50.0
	scale := 10.0

	angle := math.Pi / float64(slices)

	ctx := gg.NewContext(width, height)

	ctx.SetLineWidth(2)
	ctx.RotateAbout(gg.Radians(180), cx, cy)
	for i := 0; i < slices; i++ {
		xscale := 40 / float64(slices)
		spike := Spike()
		for p := 0; p < 5; p++ {
			pt := spike[p]
			ctx.LineTo(pt.X*xscale+cx, pt.Y*scale+cy)
		}
		if ticked > 0 {
			ctx.SetHexColor("#000")
		} else {
			ctx.SetHexColor("#fff")
		}
		ctx.FillPreserve()
		ctx.SetHexColor("#231f20")
		ctx.Stroke()
		ticked--
		ctx.RotateAbout(angle*2, cx, cy)
	}

	if err := ctx.LoadFontFace("/usr/share/fonts/noto/NotoSerif-Bold.ttf", 18); err != nil {
		panic(err)
	}

	ctx.SetHexColor("#fff")
	ctx.RotateAbout(gg.Radians(180), cx, cy)
	offset := cx + 5*scale + 10
	ctx.DrawStringWrapped(strings.ToUpper("Crows: Reestablish control of crow's foot"), offset+float64(width/3), float64(height)/2, 0.5, 0.5, float64(width)-offset, 1.1, gg.AlignCenter)

	ctx.SavePNG("out.png")
}
