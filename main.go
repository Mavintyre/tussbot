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
}

func main() {
	fmt.Println("initializing...")

	confjson, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("config.json not found")
		return
	}

	// TO DO: implement global Config var to be accessed elsewhere
	var config configJSON
	err = json.Unmarshal(confjson, &config)
	if err != nil {
		fmt.Println("JSON error", err)
		return
	}

	discord, err := discordgo.New("Bot " + config.Token)
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

	fmt.Println("shutting down...")
	discord.Close()
}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	// TO DO: read from file, save and change on command
	s.UpdateStatus(0, "doin bot stuff")
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	// TO DO: remove
	if m.Content == "!test" {
		SendReply(s, m.Message, "yo waddup")
	}

	go HandleCommand(s, m.Message)
}
