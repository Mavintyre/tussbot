package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/fogleman/gg"
)

func drawCircle(ctx *gg.Context, slices float64, ticked float64, cx float64, cy float64, scale float64) {
	radius := scale * 4.5

	angle := 360.0 / (slices / ticked)

	ctx.SetLineWidth(2)

	ctx.DrawCircle(cx, cy, radius)
	ctx.SetHexColor("#40444b")
	ctx.Fill()

	ctx.MoveTo(cx, cy)
	ctx.DrawArc(cx, cy, radius, gg.Radians(-90), gg.Radians(angle-90))
	ctx.SetHexColor("#7289da")
	ctx.Fill()

	ctx.SetHexColor("#202225")
	ctx.DrawCircle(cx, cy, radius)
	for i := 0.0; i < slices; i++ {
		x := cx + radius*math.Cos(2.0*math.Pi*i/slices)
		y := cy + radius*math.Sin(2.0*math.Pi*i/slices)
		ctx.DrawLine(cx, cy, x, y)
	}
	ctx.Stroke()
}

type point struct {
	X, Y float64
}

func spike() []point {
	poly := make([]point, 5)
	poly[0] = point{0, 0}
	poly[1] = point{-1, 1}
	poly[2] = point{0, 5}
	poly[3] = point{1, 1}
	poly[4] = point{0, 0}
	return poly
}

func drawSpikes(ctx *gg.Context, slices float64, ticked float64, cx float64, cy float64, scale float64) {
	angle := math.Pi / float64(slices)

	ctx.SetLineWidth(2)
	ctx.RotateAbout(gg.Radians(180), cx, cy)
	for i := 0.0; i < slices; i++ {
		xscale := 40 / float64(slices)
		single := spike()
		for p := 0; p < 5; p++ {
			pt := single[p]
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
}

func createClock(style string, slices float64, ticked float64, text string) (io.Reader, error) {
	width, height := 300, 100
	cx, cy := 50.0, 50.0
	scale := 10.0

	ctx := gg.NewContext(width, height)

	if style == "circle" {
		drawCircle(ctx, slices, ticked, cx, cy, scale)
	} else if style == "spikes" {
		drawSpikes(ctx, slices, ticked, cx, cy, scale)
		ctx.RotateAbout(gg.Radians(180), cx, cy) // flip canvas back the right way around
	}

	if err := ctx.LoadFontFace("/usr/share/fonts/truetype/noto/NotoSerif-Bold.ttf", 18); err != nil {
		if err := ctx.LoadFontFace("/usr/share/fonts/noto/NotoSerif-Bold.ttf", 18); err != nil {
			return nil, errors.New("unable to load font")
		}
	}

	ctx.SetHexColor("#fff")
	offset := cx + 5*scale + 10
	ctx.DrawStringWrapped(strings.ToUpper(text), offset+float64(width/3),
		float64(height)/2, 0.5, 0.5, float64(width)-offset, 1.1, gg.AlignCenter)

	buf := new(bytes.Buffer)
	ctx.EncodePNG(buf)

	return buf, nil
}

func init() {
	// TO DO: save/load clockstyle for guild
	clockStyle := "circle"

	RegisterCommand(Command{
		aliases: []string{"clockstyle"},
		help: `set clock style\n
		valid styles:
		 - ^circle^
		 - ^spikes^`,
		roles: []string{"gm"},
		callback: func(ca CommandArgs) bool {
			if ca.args != "circle" && ca.args != "spikes" {
				SendError(ca, "not a valid clock style\nsee ^%Phelp clockstyle^ for valid styles")
				return false
			}
			clockStyle = ca.args
			QuickEmbed(ca, QEmbed{content: "clock style set"})
			return false
		}})

	RegisterCommand(Command{
		aliases: []string{"clock"},
		help: `display or manipulate a clock\n
		^%Pclock something happens^ - display a clock by name
		^%Pclock someth^ - display a clock by partial name
		^%Pclock name 4^ - create clock with 4 slices
		^%Pclock name 1/4^ - create or update clock with 1/4 slices
		^%Pclock name +2^ - increase clock by 2 tick
		^%Pclock name -1^ - decrease clock by 1 tick
		^%Pclock name delete^ - delete a clock
		^%Pclock name del^ - delete a clock^`,
		roles: []string{"gm"},
		callback: func(ca CommandArgs) bool {
			buf, err := createClock(clockStyle, 4, 1, "something happens")
			if err != nil {
				return false
			}

			clockName := fmt.Sprintf("clock_%s.png", time.Now())
			ca.sess.ChannelFileSend(ca.msg.ChannelID, clockName, buf)
			return false
		}})

	RegisterCommand(Command{
		aliases:  []string{"clocks"},
		help:     `display all clocks`,
		roles:    []string{"gm"},
		emptyArg: true,
		callback: func(ca CommandArgs) bool {
			return false
		}})
}
