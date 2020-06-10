package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fogleman/gg"
)

var clockWidth, clockHeight = 300, 100

func drawCircle(ctx *gg.Context, slices float64, ticked float64, cx float64, cy float64, scale float64) {
	radius := scale * 4.5
	angle := math.Pi * (ticked / slices) * 2 // unsure why this is *2

	// rotate -90deg ccw so the starting slice is at the top
	ctx.RotateAbout(gg.Radians(-90), cx, cy)

	ctx.SetLineWidth(2)

	ctx.DrawCircle(cx, cy, radius)
	ctx.SetHexColor("#40444b")
	ctx.Fill()

	ctx.MoveTo(cx, cy)
	ctx.DrawArc(cx, cy, radius, 0, angle)
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

	// reset canvas rotation
	ctx.RotateAbout(gg.Radians(90), cx, cy)
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

	// flip canvas back the right way around
	ctx.RotateAbout(gg.Radians(180), cx, cy)
}

func writePNG(ctx *gg.Context) io.Reader {
	buf := new(bytes.Buffer)
	ctx.EncodePNG(buf)
	return buf
}

func createClock(style string, slices float64, ticked float64, text string) (*gg.Context, error) {
	width, height := clockWidth, clockHeight
	cx, cy := 50.0, 50.0
	scale := 10.0

	ctx := gg.NewContext(width, height)

	if style == "circle" {
		drawCircle(ctx, slices, ticked, cx, cy, scale)
	} else if style == "spikes" {
		drawSpikes(ctx, slices, ticked, cx, cy, scale)
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

	return ctx, nil
}

func createComposite(clocks []*clock, style string) (*gg.Context, error) {
	numClocks := len(clocks)
	perRow := 3

	width := clockWidth * perRow
	if numClocks < perRow {
		width = clockWidth * numClocks
	}
	height := clockHeight * int(math.Ceil(float64(numClocks)/float64(perRow)))

	ctx := gg.NewContext(width, height)
	// transparent is nice but background is needed so you
	// can see what you're looking at if you click to zoom
	ctx.SetHexColor("#36393f") // dark discord bg
	ctx.DrawRectangle(0, 0, float64(width), float64(height))
	ctx.Fill()

	x, y := 0, 0
	onRow := 0
	for _, cl := range clocks {
		c, err := createClock(style, float64(cl.Slices), float64(cl.Ticked), cl.Name)
		if err != nil {
			return nil, err
		}
		ctx.DrawImage(c.Image(), x, y)
		x += clockWidth
		onRow++
		if onRow == perRow {
			onRow = 0
			y += clockHeight
			x = 0
		}
	}

	return ctx, nil
}

type clock struct {
	Name   string
	Slices int
	Ticked int
}

type guildClockSettings struct {
	Clocks []*clock
	Style  string
}

var clockSettingsCache = make(map[string]*guildClockSettings)

func loadClockSettings() {
	js, err := ioutil.ReadFile("./settings/clocks.json")
	if err == nil {
		err = json.Unmarshal(js, &clockSettingsCache)
		if err != nil {
			fmt.Println("JSON error in clocks.json", err)
		}
	} else {
		fmt.Println("Unable to read clocks.json, using empty")
	}
}

func saveClockSettings() {
	// save json
	b, err := json.Marshal(clockSettingsCache)
	if err != nil {
		fmt.Println("Error marshaling JSON for clocks.json", err)
		return
	}
	err = ioutil.WriteFile("./settings/clocks.json", b, 0644)
	if err != nil {
		fmt.Println("Error saving clocks.json", err)
		return
	}
}

func guildSettings(gid string) *guildClockSettings {
	_, ok := clockSettingsCache[gid]
	if !ok {
		fmt.Println(clockSettingsCache)
		clockSettingsCache[gid] = &guildClockSettings{}
		g := clockSettingsCache[gid]
		g.Style = "circle"
	}
	return clockSettingsCache[gid]
}

func getClock(gid string, name string) *clock {
	name = strings.ToLower(name)
	gset := guildSettings(gid)
	for _, c := range gset.Clocks {
		cn := strings.ToLower(c.Name)
		if cn == name || strings.HasPrefix(cn, name) {
			return c
		}
	}
	return nil
}

func init() {
	loadClockSettings()

	RegisterCommand(Command{
		aliases: []string{"clockstyle"},
		help: `set clock style\n
		valid styles:
		 - ^circle^
		 - ^spikes^`,
		roles: []string{"gm", "botadmin"},
		callback: func(ca CommandArgs) bool {
			if ca.args != "circle" && ca.args != "spikes" {
				SendError(ca, "not a valid clock style\nsee ^%Phelp clockstyle^ for valid styles")
				return false
			}

			gset := guildSettings(ca.msg.GuildID)
			gset.Style = ca.args

			QuickEmbed(ca, QEmbed{content: "clock style set"})
			saveClockSettings()
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
		^%Pclock name del^ - delete a clock`,
		callback: func(ca CommandArgs) bool {
			// parse argument string
			fields := strings.Fields(ca.args)
			last := fields[len(fields)-1]

			action := "show"
			if last != ca.args {
				if last == "del" || last == "delete" {
					action = "delete"
				} else if regexp.MustCompile(`[+-]\d+`).MatchString(last) {
					action = "offset"
				} else if regexp.MustCompile(`(\d+\/\d+|\d+)`).MatchString(last) {
					action = "create"
				}
			}

			// only allow the gm to manipulate
			if action != "show" && !HasRole(ca.sess, ca.msg.Member, "gm") {
				return false
			}

			name := strings.Join(fields, " ")
			if action != "show" {
				name = strings.Join(fields[:len(fields)-1], " ")
			}

			// get clock
			var cl *clock
			gcl := getClock(ca.msg.GuildID, name)
			if gcl == nil {
				// don't error on nil clock if we're making a new one anyway
				if action != "create" {
					SendError(ca, "clock not found")
					return false
				}
			}
			// set clock if not nil
			if gcl != nil {
				cl = gcl
			}

			// handle actions
			if action == "delete" {
				gset := guildSettings(ca.msg.GuildID)
				for i, c := range gset.Clocks {
					if c.Name == cl.Name {
						gset.Clocks = append(gset.Clocks[:i], gset.Clocks[i+1:]...)
						break
					}
				}
				saveClockSettings()
				QuickEmbed(ca, QEmbed{content: fmt.Sprintf("`%s (%d/%d)` deleted", cl.Name, cl.Ticked, cl.Slices)})
				return false // don't show clock afterwards
			} else if action == "offset" {
				offset, err := strconv.Atoi(last)
				if err != nil {
					SendError(ca, "couldn't parse offset")
					return false
				}

				cl.Ticked = ClampI(cl.Ticked+offset, 0, cl.Slices)
				saveClockSettings()
			} else if action == "create" {
				ticked := 0
				slices := 4

				// parse slices & ticked
				rx := regexp.MustCompile(`(\d+)\/(\d+)`)
				if rx.MatchString(last) { // "1/4"
					strTicked := rx.FindAllStringSubmatch(last, -1)[0][1]
					strSlices := rx.FindAllStringSubmatch(last, -1)[0][2]

					iSlices, err := strconv.Atoi(strSlices)
					if err != nil {
						SendError(ca, "couldn't parse slice count")
						return false
					}

					iTicked, err := strconv.Atoi(strTicked)
					if err != nil {
						SendError(ca, "couldn't parse ticked count")
						return false
					}

					slices = iSlices
					ticked = iTicked
				} else if regexp.MustCompile(`\d+`).MatchString(last) { // "4" = 0/4
					lastI, err := strconv.Atoi(last)
					if err != nil {
						SendError(ca, "couldn't parse slice count")
						return false
					}
					slices = lastI
				}

				if cl != nil {
					// update existing clock
					cl.Ticked = ticked
					cl.Slices = slices
				} else {
					// create new clock
					cl = &clock{Name: name, Ticked: ticked, Slices: slices}
					gset := guildSettings(ca.msg.GuildID)
					gset.Clocks = append(gset.Clocks, cl)
				}
				saveClockSettings()
			}

			// return clock
			ctx, err := createClock(guildSettings(ca.msg.GuildID).Style, float64(cl.Slices), float64(cl.Ticked), cl.Name)
			if err != nil {
				SendError(ca, fmt.Sprintf("error creating clock: %s", err))
				return false
			}
			ca.sess.ChannelFileSend(ca.msg.ChannelID, fmt.Sprintf("clock_%s.png", time.Now()), writePNG(ctx))

			return false
		}})

	RegisterCommand(Command{
		aliases:  []string{"clocks"},
		help:     `display all clocks`,
		emptyArg: true,
		callback: func(ca CommandArgs) bool {
			gset := guildSettings(ca.msg.GuildID)
			if len(gset.Clocks) < 1 {
				SendError(ca, "no clocks in this guild")
				return false
			}

			// display composite
			ctx, err := createComposite(gset.Clocks, gset.Style)
			if err != nil {
				SendError(ca, fmt.Sprintf("error creating composite: %s", err))
				return false
			}
			ca.sess.ChannelFileSend(ca.msg.ChannelID, fmt.Sprintf("clock_%s.png", time.Now()), writePNG(ctx))
			return false
		}})
}
