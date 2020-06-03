package main

import (
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
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
		return nil, fmt.Errorf("probability too high to compute")
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
	// TO DO: command to output current seed
	seed := time.Now().UnixNano()
	rand.Seed(seed)

	RegisterCommand(Command{
		aliases: []string{"roll", "r"},
		callback: func(s *discordgo.Session, m *discordgo.Message, args string) {
			if !regexp.MustCompile(`^(\d+d\d+\s?)+$`).MatchString(args) {
				SendReply(s, m, "invalid roll parameters") // TO DO: use SendError
				return
			}

			var results [][]int
			for _, str := range strings.Fields(args) {
				res, err := rollDice(str)
				if err != nil {
					SendReply(s, m, err.Error()) // TO DO: use SendError
					return
				}
				sum := res[0] + res[1]
				s := []int{sum, sum}
				results = append(results, s)
			}

			// TO DO: improve output
			_, err := SendEmbed(s, m, &discordgo.MessageEmbed{Description: fmt.Sprintf("`%v`", results)})
			if err != nil {
				fmt.Println(err)
			}
		},
	})

	RegisterCommand(Command{
		aliases: []string{"shake", "seed", "reseed"},
		callback: func(s *discordgo.Session, m *discordgo.Message, args string) {
			seed := time.Now().UnixNano()
			rand.Seed(seed)
			SendReply(s, m, "seed has been shakened")
			// TO DO: improve output, use a random 'shake' gif? output new seed
		},
	})
}
