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

			// regex for a single roll expression
			singleRx := `\d*d\d+[b!]?`
			singleRxC := regexp.MustCompile(singleRx)

			str := strings.ToLower(ca.args)

			if !singleRxC.MatchString(str) {
				SendError(ca, "no valid roll expressions found")
				return false
			}

			// check for gm flag and remove if present
			isGMRoll := false
			if strings.HasPrefix(str, "gm") {
				isGMRoll = true
				str = str[3:]
			}

			// capture tags and remove if present
			groups := regexp.MustCompile(`(?:\d|`+singleRx+`)(\s+[\w ]+?$)`).FindAllStringSubmatch(str, -1)
			tags := ""
			if len(groups) > 0 && len(groups[0]) > 0 {
				tags = groups[0][1]
				str = strings.ReplaceAll(str, tags, "")
				tags = strings.TrimSpace(tags)
			}

			// replace space with +
			str = strings.ReplaceAll(str, " ", "+")

			// remove all spaces
			str = strings.ReplaceAll(str, " ", "")

			// replace tokens with just one
			str = regexp.MustCompile(`\++`).ReplaceAllString(str, "+")
			str = regexp.MustCompile(`-+`).ReplaceAllString(str, "-")

			// add a space before + or -
			str = strings.ReplaceAll(str, "+", " + ")
			str = strings.ReplaceAll(str, "-", " - ")

			// replace excess spaces with just one
			str = regexp.MustCompile(`\s+`).ReplaceAllString(str, " ")

			var results string

			// split into sets of expressions and iterate
			sum := 0
			lastOp := "+"
			lastNum := 0
			sets := strings.Split(str, " ")
			for _, expr := range sets {
				if expr == "+" || expr == "-" {
					lastOp = expr
					results += fmt.Sprintf(" *%s* ", expr)
					continue
				}
				if !singleRxC.MatchString(expr) {
					results += "*" + expr + "*"
					num, err := strconv.Atoi(expr)
					if err != nil {
						SendError(ca, fmt.Sprintf("could not parse modifier: %s", err))
						return false
					}
					lastNum = num
				} else {
					// assume 1 dice if no number of dice specified before 'd'
					if expr[0] == 'd' {
						expr = "1" + expr
					}

					// check for exploding and boon tokens
					isExploding := false
					isBoon := false
					for strings.HasSuffix(expr, "!") || strings.HasSuffix(expr, "b") {
						if strings.HasSuffix(expr, "!") {
							isExploding = true
							expr = strings.Replace(expr, "!", "", -1)
						}
						if strings.HasSuffix(expr, "b") {
							isBoon = true
							expr = strings.Replace(expr, "b", "", -1)
						}
					}

					// split into numDice and diceVal
					split := strings.Split(expr, "d")
					if len(split) < 2 {
						SendError(ca, fmt.Sprintf("not enough parameters in roll syntax"))
						return false
					}

					numDice, err := strconv.Atoi(split[0])
					if err != nil {
						SendError(ca, fmt.Sprintf("couldn't parse number of dice: %s", err))
						return false
					}

					diceVal, err := strconv.Atoi(split[1])
					if err != nil {
						SendError(ca, fmt.Sprintf("couldn't parse dice value: %s", err))
						return false
					}

					// roll values
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

					// handle boon rolls
					var rejectedVals []int
					if isBoon {
						highest := 0
						for _, num := range vals {
							if num > highest {
								highest = num
							} else {
								rejectedVals = append(rejectedVals, num)
							}
						}
						vals = nil
						vals = append(vals, highest)
					}

					// handle exploding die
					if isExploding {
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

					// format rolled values
					results += "("
					if isBoon {
						for _, num := range rejectedVals {
							results += fmt.Sprintf("~~`[%d]`~~ ", num)
						}
					}
					for i, num := range vals {
						wrap := ""
						spacer := ""
						suffix := ""
						// spacer between values
						// separate exploded values
						if i == numDice {
							spacer = " **!** "
						} else if i != 0 {
							spacer = " "
						}
						// make exploded values italic
						// and have a ! suffix
						if i > numDice-1 {
							wrap = "*"
							suffix = "!"
						}
						// make crits bold
						if num == diceVal || num == 1 {
							wrap += "**"
						}
						// add to results string
						results += fmt.Sprintf("%s%s`[%d%s]`%s", spacer, wrap, num, suffix, wrap)
					}
					results += ")"

					// sum the set
					total := 0
					for _, num := range vals {
						total += num
					}
					lastNum = total
				}
				if lastOp == "+" {
					sum += lastNum
				} else if lastOp == "-" {
					sum -= lastNum
				}
			}

			results += fmt.Sprintf(" *= **%d***", sum)

			qem := QEmbed{content: results, footer: tags}
			qem.title = fmt.Sprintf("roll by %s", GetNick(ca.msg.Member))

			// handle gm roll
			if isGMRoll {
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
		roles:    []string{"botadmin", "gm"},
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
