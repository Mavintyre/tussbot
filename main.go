package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

type configJSON struct {
	Token   string
	OwnerID string
	Prefix  string
}

// Config JSON
var Config configJSON

func main() {
	fmt.Println("Initializing...")

	confjson, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("Unable to read config.json")
		return
	}

	err = json.Unmarshal(confjson, &Config)
	if err != nil {
		fmt.Println("JSON error", err)
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
	// TO DO: read from config, save and change on command
	s.UpdateStatus(0, "doin bot stuff")
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	// TO DO: is 'go' here necessary? (lib should spawn messageCreate with go)
	//	test to see if there's stalling, once ffmpeg is integrated
	HandleCommand(s, m.Message)
}
