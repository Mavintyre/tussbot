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

func rollDice(input string) ([]int, error) {
	split := strings.Split(input, "d")
	numDice, _ := strconv.ParseFloat(split[0], 64)
	diceVal, _ := strconv.Atoi(split[1])

	if numDice < 1 || diceVal <= 1 {
		return nil, errors.New("nothing to roll")
	}

	maxProb := math.Pow(float64(diceVal), numDice)
	if int64(maxProb-1) < 0 {
		return nil, errors.New("probability too high to compute")
	}

	valSpan := maxProb / float64(diceVal)

	table := probTable{}
	table.init(diceVal, valSpan)

	var results []int
	for d := 0; d < int(numDice); d++ {
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
		^!roll gm 2d6^ - roll that only you and the GM can see
		^$roll 2d6 3d20^ - roll multiple sets of dice
		^!roll 1dS^ - roll custom dice of name S`,
		callback: func(ca CommandArgs) {
			// TO DO: keep stats of rolls cumulative & per user
			//	- distribution, set runs, consequtive runs
			// TO DO: allow omission of number of rolls to default to 1
			// TO DO: optimization for 1d6, don't use probtable, just generate 1-6
			// TO DO: exploding die: on max roll, add another die (no table)
			//		- !roll 2d6x
			// TO DO: 2d6+1 or 2d6-1 syntax for bonus/minus die
			// TO DO: custom die
			//		- roll as name !roll 2dZ or 2dZoop
			//		- !setdie name "a" "b" "c"
			//		- !setface diename facename some string (image attachment)
			//		- !deletedie name
			//		- keep struct of die > face > emojiID (per guild)
			//		- if deleting die, remove emojis
			//		- if !set a die, remove emojis for faces that no longer exist (by name or index?)
			//		- if !setface remove all old emoji -- if no new emoji is given, emoji is removed
			//

			// TO DO: gm roll
			//	- get first member of gm role in channel
			//	- if no gm role or gm member found, error
			//	- command to set gm role name
			//	- store role id in per-guild json
			//	- if gm rolls, send only to gm
			//	- if player rolls, send to gm and player
			//	- !setgmrole gm
			//	- !delgmrole
			//	- !roll gm 2d6

			regex := regexp.MustCompile(`^(\d+d\d+\s?)+( [\w ]+)?$`)
			if !regex.MatchString(ca.args) {
				SendError(ca, "invalid roll parameters\ncheck `%Phelp roll` for usage")
				return
			}

			groups := regex.FindAllStringSubmatch(ca.args, -1)
			if len(groups) == 0 || len(groups[0]) < 3 {
				SendError(ca, "error matching regex")
				return
			}
			rollstr := groups[0][1]
			tags := groups[0][2]

			var results []string
			for _, str := range strings.Fields(rollstr) {
				var setres []string
				vals, err := rollDice(str)
				if err != nil {
					SendError(ca, err.Error())
					return
				}
				for _, die := range vals {
					setres = append(setres, fmt.Sprintf("`[%v]`", die))
				}
				results = append(results, strings.Join(setres, " "))
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
