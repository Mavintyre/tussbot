package main

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// CommandList of chat commands
var CommandList []Command

// Command definition and parameters
type Command struct {
	aliases  []string
	callback func(*discordgo.Session, *discordgo.Message, string)
}

// RegisterCommand to the bot
func RegisterCommand(cmd Command) {
	CommandList = append(CommandList, cmd)
}

// HandleCommand on message event
func HandleCommand(s *discordgo.Session, m *discordgo.Message) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(fmt.Errorf("<Recovered panic in HandleCommand>\n%s", PanicStack()))
		}
	}()

	if !strings.HasPrefix(m.Content, "!") {
		return
	}
	split := strings.SplitN(m.Content[1:], " ", 2)
	mname := split[0]

	margs := ""
	if len(split) > 1 {
		margs = split[1]
	}

	for _, cmd := range CommandList {
		for _, a := range cmd.aliases {
			if a == mname {
				cmd.callback(s, m, margs)
			}
		}
	}
}

// SendReply to a message's source channel with a string -- returns message and error
func SendReply(s *discordgo.Session, m *discordgo.Message, str string) (*discordgo.Message, error) {
	str = StrMax(str, 2000)

	nm, err := s.ChannelMessageSend(m.ChannelID, str)
	if err != nil {
		err = fmt.Errorf("error sending reply: %w", err)
		SendError(s, m, err.Error())
	}
	return nm, err
}

// SendEmbed to a message's source channel with an embed
func SendEmbed(s *discordgo.Session, m *discordgo.Message, em *discordgo.MessageEmbed) (*discordgo.Message, error) {
	em.Description = StrMax(em.Description, 2048)

	nm, err := s.ChannelMessageSendEmbed(m.ChannelID, em)
	if err != nil {
		err = fmt.Errorf("error sending embed: %w", err)
		SendError(s, m, err.Error())
	}
	return nm, err
}

// SendError to a message's source channel with special error formatting
func SendError(s *discordgo.Session, m *discordgo.Message, str string) {
	// not using SendEmbed here so we don't get stuck in a SendError loop
	_, err := s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{Title: "error", Description: StrMax(str, 2000), Color: 0xff0000})
	if err != nil {
		err = fmt.Errorf("error sending error: %w", err)
		fmt.Println(err)
	}
}
