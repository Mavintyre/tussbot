package main

import (
	"math"

	"github.com/fogleman/gg"
)

func drawClock() {
	width, height := 100, 100
	cx, cy := 50.0, 50.0
	radius := 30.0
	border := 2.0

	slices := 8.0
	ticked := 7.0

	angle := 360 / (slices / ticked)

	ctx := gg.NewContext(width, height)

	ctx.DrawCircle(cx, cy, radius)
	ctx.SetRGB(1, 1, 1)
	ctx.Fill()

	ctx.MoveTo(cx, cy)
	ctx.DrawArc(cx, cy, radius, gg.Radians(-90), gg.Radians(angle-90))
	ctx.SetRGB(1.0, 0, 0)
	ctx.Fill()

	ctx.SetRGB(0, 0, 0)
	ctx.SetLineWidth(border)
	ctx.DrawCircle(cx, cy, radius)
	for i := 0.0; i < slices; i++ {
		x := cx + radius*math.Cos(2.0*math.Pi*i/slices)
		y := cy + radius*math.Sin(2.0*math.Pi*i/slices)
		ctx.DrawLine(cx, cy, x, y)
	}
	ctx.Stroke()

	ctx.SavePNG("circle2.png")
}
