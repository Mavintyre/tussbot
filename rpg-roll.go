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
		^%Proll d6^ - roll a dice with 6 faces
		^%Proll 2d6^ - roll 2 dice with 6 faces
		^%Proll 2d6#^ - roll 2d6 and show their sum
		^%Proll 2d6+1^ - roll 2d6 with a modifier (always shows sum)
		^%Proll 2d6!^ - roll 2d6 with exploding re-rolls
		^%Proll gm 2d6^ - roll that only you and the GM can see
		^%Proll 2d6 3d20^ - roll multiple sets of dice
		^%Proll 2d6 risky standard^ - tag a roll's output`,
		//^%Proll 1dS^ - roll custom dice of name S`,
		callback: func(ca CommandArgs) bool {
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

			// uber regex to check for valid syntax
			regex := regexp.MustCompile(`^((?:gm )?(?:\d*d\d+(?:[+-]\d+)?(?:[!#]+)?\s?)+)\s?([\w ]+)?$`)
			if !regex.MatchString(ca.args) {
				SendError(ca, "invalid roll parameters\ncheck `%Phelp roll` for usage")
				return false
			}

			// gm roll
			gmRoll := false
			if strings.HasPrefix(ca.args, "gm") {
				gmRoll = true
				ca.args = strings.ReplaceAll(ca.args, "gm ", "")
			}

			// separate roll syntax from tags
			// TO DO: is it necessary to check length on groups if MustCompile?
			groups := regex.FindAllStringSubmatch(ca.args, -1)
			if len(groups) == 0 || len(groups[0]) < 3 {
				SendError(ca, "error matching regex")
				return false
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

				// sum modifier
				modifier := 0
				modreg := regexp.MustCompile(`([+-]\d+)`)
				if modreg.MatchString(str) {
					modmatch := modreg.FindAllString(str, -1)
					// TO DO: is it necessary to check length on groups if MustCompile?
					if len(modmatch) < 1 {
						SendError(ca, "error capturing modifier syntax")
						return false
					}
					modstr := modmatch[0]
					str = strings.Replace(str, modstr, "", -1)
					mod, err := strconv.Atoi(modstr)
					if err != nil {
						SendError(ca, "couldn't parse modifier"+err.Error())
						return false
					}
					modifier = mod
				}

				// split into value and number of die
				split := strings.Split(str, "d")
				if len(split) < 2 {
					SendError(ca, "error parsing roll string")
					return false
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

				// imply # when using sum modifier
				if modifier > 0 {
					getSum = true
				}

				// parse number of dice and value
				numDice, err := strconv.Atoi(split[0])
				if err != nil {
					// TO DO: replace all instances of the following with sprintf
					//			""+err.Error
					//			err.Error
					//			"",err
					SendError(ca, "couldn't parse number of dice: "+err.Error())
					return false
				}
				diceVal, err := strconv.Atoi(split[1])
				if err != nil {
					SendError(ca, "couldn't parse dice value: "+err.Error())
					return false
				}

				// roll die
				var vals []int
				if numDice == 1 { // don't bother setting up a ProbTable for just 1 die
					num := int(rand.Int63n(int64(diceVal)) + 1)
					vals = append(vals, num)
				} else { // roll with ProbTable
					rollVals, err := rollDice(numDice, diceVal)
					if err != nil {
						SendError(ca, err.Error())
						return false
					}
					vals = append(vals, rollVals...)
				}

				// handle exploding die
				if exploding {
					for _, num := range vals {
						if num == diceVal {
							newNum := int(rand.Int63n(int64(diceVal)) + 1)
							vals = append(vals, newNum)
							for newNum == diceVal {
								newNum = int(rand.Int63n(int64(diceVal)) + 1)
								vals = append(vals, newNum)
							}

						}
					}
				}

				// handle rolled values
				for i, num := range vals {
					wrap := ""
					spacer := ""
					if i == numDice {
						spacer = " ! "
					} else {
						spacer = ""
					}
					if i > numDice-1 {
						wrap = "*"
					}
					if num == diceVal {
						wrap += "**"
					}
					setres = append(setres, fmt.Sprintf("%s%s`[%v]`%s", spacer, wrap, num, wrap))
				}

				// sum it up
				sum := ""
				if getSum {
					total := 0
					for _, num := range vals {
						total += num
					}
					if modifier > 0 {
						total += modifier
						sum = fmt.Sprintf(" *+ %d = **%d***", modifier, total)
					} else {
						sum = fmt.Sprintf(" *= %d*", total)
					}
				}

				results = append(results, strings.Join(setres, " ")+sum)
			}

			retstr := results[0]
			if len(results) > 1 {
				retstr = strings.Join(results, "  **--**  ")
			}
			qem := QEmbed{content: retstr, footer: tags}
			qem.title = fmt.Sprintf("roll by %s", GetNick(ca.msg.Member))

			// handle gm roll
			if gmRoll {
				// find gm in channel
				gms, err := FindMembersByRole(ca.sess, ca.msg.GuildID, "gm")
				if err != nil {
					SendError(ca, fmt.Sprintf("error finding gm: %s", err))
					return false
				}
				if len(gms) < 1 {
					SendError(ca, "no gm found in this server")
					return false
				}

				// dm the gm
				chG, err := GetDMChannel(ca.sess, gms[0].User.ID)
				if err != nil {
					SendError(ca, fmt.Sprintf("error DMing gm: %s", err))
					return false
				}
				QuickEmbed(CommandArgs{sess: ca.sess, chO: chG.ID}, qem)

				// dm the user
				chU, err := GetDMChannel(ca.sess, ca.msg.Author.ID)
				if err != nil {
					SendError(ca, fmt.Sprintf("error DMing user: %s", err))
					return false
				}
				QuickEmbed(CommandArgs{sess: ca.sess, chO: chU.ID}, qem)

				return false
			}

			QuickEmbed(ca, qem)
			return false
		},
	})

	RegisterCommand(Command{
		aliases: []string{"seed"},
		help: `display or change random seed\n
		^%Pseed^ - display current seed
		^%Pseed asdf^ - change seed to "asdf"`,
		emptyArg: true,
		noDM:     true,
		roles:    []string{"botadmin"},
		callback: func(ca CommandArgs) bool {
			if ca.args == "" && ca.alias == "seed" {
				QuickEmbed(ca, QEmbed{content: fmt.Sprintf("current seed: %v", seedstr)})
				return false
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
				return false
			}
			QuickEmbed(ca, QEmbed{title: "roll reseeded", content: content})
			return false
		},
	})
}
