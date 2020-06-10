package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

type configJSON struct {
	Token          string
	OwnerID        string
	Prefixes       []string
	PrefixOptional bool
	Status         string
}

// Config JSON
var Config configJSON

func main() {
	fmt.Println("Initializing...")

	confjson, err := ioutil.ReadFile("./settings/config.json")
	if err != nil {
		fmt.Println("Unable to read config.json")
		return
	}

	err = json.Unmarshal(confjson, &Config)
	if err != nil {
		fmt.Println("JSON error in config.json", err)
		return
	}

	discord, err := discordgo.New("Bot " + Config.Token)
	if err != nil {
		fmt.Println("Error creating Discord session", err)
		return
	}

	discord.AddHandler(ready)
	discord.AddHandler(messageCreate)

	err = discord.Open()
	if err != nil {
		fmt.Println("Error opening Discord session", err)
		return
	}

	fmt.Println("TussBot initialized")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	fmt.Println("Shutting down...")
	discord.Close()
}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	if Config.Status != "" {
		s.UpdateStatus(0, Config.Status)
	}
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	// "go" is unnecessary here as lib already calls "go messageCreate..."
	HandleCommand(s, m.Message)
}

func init() {
	RegisterCommand(Command{
		aliases:   []string{"stats"},
		help:      "bot runtime stats",
		emptyArg:  true,
		ownerOnly: true,
		callback: func(ca CommandArgs) bool {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			mbAlloc := float64(m.Alloc) / 1024 / 1024
			mbStack := float64(m.StackSys) / 1024 / 1024
			pauseTime := float64(m.PauseNs[(m.NumGC+255)%256] / 1000000)
			numRoutines := runtime.NumGoroutine()
			stats := fmt.Sprintf("`alloc: %.2fMB`\n`stack: %.2fMB`\n`pause: %.2fms`\n`numgo: %d`", mbAlloc, mbStack, pauseTime, numRoutines)
			QuickEmbed(ca, QEmbed{title: "runtime stats", content: stats})
			return false
		}})

	RegisterCommand(Command{
		aliases:   []string{"setstatus", "status"},
		help:      "set bot status",
		ownerOnly: true,
		callback: func(ca CommandArgs) bool {

			status := ca.args
			ca.sess.UpdateStatus(0, status)

			Config.Status = status

			// TO DO: helper func for writing bytes to JSON
			b, err := json.MarshalIndent(Config, "", "\t")
			if err != nil {
				fmt.Println("Error marshaling JSON for config.json", err)
				return false
			}
			err = ioutil.WriteFile("./settings/config.json", b, 0644)
			if err != nil {
				fmt.Println("Error saving config.json", err)
				return false
			}

			return false
		}})
}
