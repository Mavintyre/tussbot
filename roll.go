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
		callback: func(ca CommandArgs) {
			// TO DO: allow omission of number of rolls to default to 1

			// TO DO: gm roll
			//	- get first member of gm role in channel
			//	- if no gm role or gm member found, error
			//	- command to set gm role name
			//	- store role id in per-guild json

			// TO DO: generic extra args that are reiterated as tags in the output

			if !regexp.MustCompile(`^(\d+d\d+\s?)+$`).MatchString(ca.args) {
				SendError(ca, "invalid roll parameters")
				return
			}

			var results []string
			for _, str := range strings.Fields(ca.args) {
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

			if len(results) == 1 {
				QuickEmbed(ca, results[0])
			} else {
				QuickEmbed(ca, strings.Join(results, "  **--**  "))
			}
		},
	})

	RegisterCommand(Command{
		aliases: []string{"seed", "reseed"},
		callback: func(ca CommandArgs) {
			if ca.args == "" && ca.alias == "seed" {
				QuickEmbed(ca, fmt.Sprintf("current seed: %v", seedstr))
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
				QuickEmbedTF(ca, "roll reseeded", content, footer)
				return
			}
			QuickEmbedT(ca, "roll reseeded", content)
		},
	})
}
