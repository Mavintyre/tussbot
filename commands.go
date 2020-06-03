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
	err := checkLength("reply content", len(str), 2000)
	if err != nil {
		SendError(s, m, err.Error())
		return nil, err
	}

	nm, err := s.ChannelMessageSend(m.ChannelID, str)
	if err != nil {
		err = fmt.Errorf("error sending reply: %w", err)
		SendError(s, m, err.Error())
		fmt.Println(err)
	}
	return nm, err
}

// SendEmbed to a message's source channel with an embed
func SendEmbed(s *discordgo.Session, m *discordgo.Message, em *discordgo.MessageEmbed) (*discordgo.Message, error) {
	err := checkLength("embed title", len(em.Title), 256)
	if err != nil {
		SendError(s, m, err.Error())
		return nil, err
	}

	err = checkLength("embed description", len(em.Description), 2048)
	if err != nil {
		SendError(s, m, err.Error())
		return nil, err
	}

	err = checkLength("embed fields", len(em.Fields), 25)
	if err != nil {
		SendError(s, m, err.Error())
		return nil, err
	}

	if len(em.Fields) > 0 {
		for i, field := range em.Fields {
			err = checkLength(fmt.Sprintf("field #%d name", i), len(field.Name), 256)
			if err != nil {
				SendError(s, m, err.Error())
				return nil, err
			}

			err = checkLength(fmt.Sprintf("field #%d value", i), len(field.Value), 1024)
			if err != nil {
				SendError(s, m, err.Error())
				return nil, err
			}
		}
	}

	if em.Footer != nil {
		err = checkLength("footer text", len(em.Footer.Text), 2048)
		if err != nil {
			SendError(s, m, err.Error())
			return nil, err
		}
	}

	if em.Author != nil {
		err = checkLength("author name", len(em.Author.Name), 256)
		if err != nil {
			SendError(s, m, err.Error())
			return nil, err
		}
	}

	nm, err := s.ChannelMessageSendEmbed(m.ChannelID, em)
	if err != nil {
		err = fmt.Errorf("error sending embed: %w", err)
		SendError(s, m, err.Error())
		fmt.Println(err)
	}
	return nm, err
}

// SendError to a message's source channel with special error formatting
func SendError(s *discordgo.Session, m *discordgo.Message, str string) {
	_, err := s.ChannelMessageSend(m.ChannelID, str)
	if err != nil {
		err = fmt.Errorf("error sending error: %w", err)
		fmt.Println(err)
	}
}

func checkLength(key string, length int, max int) error {
	if length > max {
		err := fmt.Errorf("%s exceeds max length %d>%d", key, length, max)
		return err
	}
	return nil
}
