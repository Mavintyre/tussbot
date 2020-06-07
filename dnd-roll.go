package main

import (
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type probItem struct {
	val               int
	span, first, last float64
}

type probTable struct {
	table []probItem
	max   float64
}

func (pt *probTable) init(diceVal int, valSpan float64) {
	for i := 1; i <= diceVal; i++ {
		pt.table = append(pt.table, probItem{val: i, span: valSpan})
	}
	pt.max = float64(diceVal) * valSpan
	pt.renumerate()
}

func (pt *probTable) renumerate() {
	var i float64 = 1.0
	for k, item := range pt.table {
		pt.table[k].first = i
		pt.table[k].last = i + item.span - 1
		i += item.span
	}
	pt.max = i - 1
}

func (pt *probTable) atIndex(num float64) int {
	for _, item := range pt.table {
		if num >= item.first && num <= item.last {
			return item.val
		}
	}
	return -1
}

func (pt *probTable) reduceProb(val int) {
	for k, item := range pt.table {
		if val == item.val {
			pt.table[k].span--
			break
		}
	}
	pt.renumerate()
}

func rollDice(numDice int, diceVal int) ([]int, error) {
	if numDice < 1 || diceVal <= 1 {
		return nil, errors.New("nothing to roll")
	}

	maxProb := math.Pow(float64(diceVal), float64(numDice))
	if int64(maxProb-1) < 0 {
		return nil, errors.New("probability too high to compute")
	}

	valSpan := maxProb / float64(diceVal)

	table := probTable{}
	table.init(diceVal, valSpan)

	var results []int
	for d := 0; d < numDice; d++ {
		i := rand.Int63n(int64(table.max-1)) + 1
		num := table.atIndex(float64(i))
		results = append(results, num)
		table.reduceProb(num)
	}

	return results, nil
}

func init() {
	seed := time.Now().UnixNano()
	seedstr := strconv.Itoa(int(seed))
	rand.Seed(seed)

	RegisterCommand(Command{
		aliases: []string{"roll", "r"},
		help: `roll some dice with realistic probability\n
		^!roll d6^ - roll a dice with 6 faces
		^!roll 2d6^ - roll 2 dice with 6 faces
		^!roll 2d6+1^ - roll 2d6 with an extra die
		^!roll 2d6-1^ - roll 2d6 minus one die
		^!roll 2d6!^ - roll 2d6 with exploding re-rolls
		^!roll 2d6#^ - roll 2d6 and show their sum
		^!roll gm 2d6^ - roll that only you and the GM can see
		^$roll 2d6 3d20^ - roll multiple sets of dice
		^!roll 2d6 risky standard^ - tag a roll's output
		^!roll 1dS^ - roll custom dice of name S`,
		callback: func(ca CommandArgs) {
			// TO DO: custom die
			//  	- store in guild storage "roll/dice" scope
			//		- roll as name !roll 2dZ or 2dZoop
			//		- !setdie name "a" "b" "c"
			//		- !setface diename facename some string (image attachment)
			//		- !deletedie name
			//		- keep struct of die > face > emojiID (per guild)
			//		- if deleting die, remove emojis
			//		- if !set a die, remove emojis for faces that no longer exist (by name or index?)
			//		- if !setface remove all old emoji -- if no new emoji is given, emoji is removed

			// TO DO: gm roll
			//  - store in guild storage "roll/gm" scope
			//	- get first member of gm role in channel
			//	- if no gm role or gm member found, error
			//	- command to set gm role name
			//	- store role id in per-guild json
			//	- if gm rolls, send only to gm
			//	- if player rolls, send to gm and player
			//	- !setgmrole gm
			//	- !delgmrole
			//	- !roll gm 2d6

			// uber regex to check for valid syntax
			regex := regexp.MustCompile(`^((?:\d*d\d+(?:[+-]\d+)?(?:[!#]+)?\s?)+)\s?([\w ]+)?$`)
			if !regex.MatchString(ca.args) {
				SendError(ca, "invalid roll parameters\ncheck `%Phelp roll` for usage")
				return
			}

			// separate roll syntax from tags
			groups := regex.FindAllStringSubmatch(ca.args, -1)
			if len(groups) == 0 || len(groups[0]) < 3 {
				SendError(ca, "error matching regex")
				return
			}
			rollstr := groups[0][1]
			tags := groups[0][2]

			// for each set in rollstr
			var results []string
			for _, str := range strings.Fields(rollstr) {
				var setres []string

				// if no number before 'd', assume 1
				if str[0] == 'd' {
					str = "1" + str
				}

				// bonus die
				numoffset := 0
				extrareg := regexp.MustCompile(`([+-]\d+)`)
				if extrareg.MatchString(str) {
					extramatch := extrareg.FindAllString(str, -1)
					if len(extramatch) < 1 {
						SendError(ca, "error capturing extra die syntax")
						return
					}
					extrastr := extramatch[0]
					str = strings.Replace(str, extrastr, "", -1)
					extra, err := strconv.Atoi(extrastr)
					if err != nil {
						SendError(ca, "couldn't parse extra die"+err.Error())
						return
					}
					numoffset = extra
				}

				// split into value and number of die
				split := strings.Split(str, "d")
				if len(split) < 2 {
					SendError(ca, "error parsing roll string")
					return
				}

				// handle if tail end has exploding or sum syntax tokens
				exploding := false
				getSum := false
				for strings.HasSuffix(split[1], "!") || strings.HasSuffix(split[1], "#") {
					if strings.HasSuffix(split[1], "!") {
						exploding = true
						split[1] = strings.Replace(split[1], "!", "", -1)
					}
					if strings.HasSuffix(split[1], "#") {
						getSum = true
						split[1] = strings.Replace(split[1], "#", "", -1)
					}
				}

				// parse number of dice and value
				numDice, err := strconv.Atoi(split[0])
				if err != nil {
					SendError(ca, "couldn't parse number of dice"+err.Error())
					return
				}
				diceVal, err := strconv.Atoi(split[1])
				if err != nil {
					SendError(ca, "couldn't parse dice value"+err.Error())
					return
				}
				numDice += numoffset

				// roll die
				var vals []int
				if numDice == 1 { // don't bother setting up a ProbTable for just 1 die
					num := int(rand.Int63n(int64(diceVal)) + 1)
					vals = append(vals, num)
				} else { // roll with ProbTable
					rollVals, err := rollDice(numDice, diceVal)
					if err != nil {
						SendError(ca, err.Error())
						return
					}
					vals = append(vals, rollVals...)
				}

				// handle exploding die
				for i := 0; i < len(vals); i++ {
					num := vals[i]
					if exploding {
						for num == diceVal {
							num = int(rand.Int63n(int64(diceVal)) + 1)
							vals = append(vals, num)
						}
					}
				}

				// handle rolled values
				for i, num := range vals {
					exploded := ""
					if i > numDice-1 {
						exploded = "!"
					}
					setres = append(setres, fmt.Sprintf("`[%s%v]`", exploded, num))
				}

				// sum it up
				sum := ""
				if getSum {
					total := 0
					for _, num := range vals {
						total += num
					}
					sum = fmt.Sprintf(" *= %d*", total)
				}

				results = append(results, strings.Join(setres, " ")+sum)
			}

			retstr := results[0]
			if len(results) > 1 {
				retstr = strings.Join(results, "  **--**  ")
			}
			QuickEmbed(ca, QEmbed{content: retstr, footer: tags})
		},
	})

	RegisterCommand(Command{
		aliases: []string{"seed"},
		help: `display or change random seed\n
		^%Pseed^ - display current seed
		^%Pseed asdf^ - change seed to "asdf"`,
		emptyArg: true,
		callback: func(ca CommandArgs) {
			if ca.args == "" && ca.alias == "seed" {
				QuickEmbed(ca, QEmbed{content: fmt.Sprintf("current seed: %v", seedstr)})
				return
			}
			seed = time.Now().UnixNano()
			footer := ""
			if ca.args != "" {
				data := []byte(ca.args)
				sum := md5.Sum(data)
				seed = int64(binary.BigEndian.Uint64(sum[:]))
				seed %= time.Now().UnixNano() // crude attempt to mitigate seed restart manipulation
				footer = fmt.Sprintf("\"%s\" hashed to numerical value and\nmodulated by time to mitigate manipulation", ca.args)
			}
			rand.Seed(seed)
			seedstr = strconv.Itoa(int(seed))

			content := fmt.Sprintf("new seed: %v", seedstr)
			if footer != "" {
				QuickEmbed(ca, QEmbed{title: "roll reseeded", content: content, footer: footer})
				return
			}
			QuickEmbed(ca, QEmbed{title: "roll reseeded", content: content})
		},
	})
}
