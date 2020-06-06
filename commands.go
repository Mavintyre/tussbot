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
	callback func(CommandArgs)
}

// RegisterCommand to the bot
func RegisterCommand(cmd Command) {
	CommandList = append(CommandList, cmd)
}

// CommandArgs to be passed around easily
type CommandArgs struct {
	sess  *discordgo.Session
	msg   *discordgo.Message
	ch    string
	cmd   *Command
	alias string
	args  string
}

// HandleCommand on message event
func HandleCommand(s *discordgo.Session, m *discordgo.Message) {
	// TO DO: pm owner on panic
	defer func() {
		if r := recover(); r != nil {
			stack := PanicStack()
			fmt.Println("<Recovered panic in HandleCommand>", stack)
			ch, err := GetDMChannel(s, Config.OwnerID)
			if err != nil {
				fmt.Println("error DMing owner panic log", err)
				return
			}
			stack = strings.Replace(stack, "	", ">", -1)
			SendReply(CommandArgs{sess: s, ch: ch.ID}, fmt.Sprintf("`<Recovered panic in HandleCommand>`\n```%s```", StrClamp(stack, 1957)))
		}
	}()

	// TO DO: allow commands without prefix
	//	- how to achieve pasting urls with this? regex aliases?
	//	- DEFINITELY restrict bot to one channel when doing this
	if !strings.HasPrefix(m.Content, Config.Prefix) {
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
				cmd.callback(CommandArgs{sess: s, msg: m, args: margs, alias: mname, cmd: &cmd})
			}
		}
	}
}

// TO DO: help command
// TO DO: empty args shows help
//	- flag to toggle (on|off?) for commands that accept no args
//	- avoid handling args globally

// GetDMChannel finds a user by ID and returns a Channel for DMing them in
func GetDMChannel(s *discordgo.Session, id string) (*discordgo.Channel, error) {
	user, err := s.User(id)
	if err != nil {
		return nil, fmt.Errorf("error getting user to DM: %s", err)
	}
	ch, err := s.UserChannelCreate(user.ID)
	if err != nil {
		return nil, fmt.Errorf("error creating DM channel: %w", err)
	}
	return ch, nil
}

// SendReply to a message's source channel with a string -- returns message and error
func SendReply(ca CommandArgs, str string) (*discordgo.Message, error) {
	str = StrClamp(str, 2000)

	ch := ""
	if ca.ch != "" {
		ch = ca.ch
	} else {
		ch = ca.msg.ChannelID
	}

	nm, err := ca.sess.ChannelMessageSend(ch, str)
	if err != nil {
		err = fmt.Errorf("error sending reply in %s: %w", GetChannelName(ca.sess, ch), err)
		SendError(ca, err.Error())
	}
	return nm, err
}

// SendEmbed to a message's source channel with an embed
func SendEmbed(ca CommandArgs, em *discordgo.MessageEmbed) (*discordgo.Message, error) {
	em.Title = StrClamp(em.Title, 256)
	em.Description = StrClamp(em.Description, 2048)

	for len(em.Fields) > 25 {
		em.Fields = em.Fields[:len(em.Fields)-1]
	}

	for _, field := range em.Fields {
		field.Name = StrClamp(field.Name, 256)
		field.Value = StrClamp(field.Value, 1024)
	}

	if em.Footer != nil {
		em.Footer.Text = StrClamp(em.Footer.Text, 2048)
	}

	if em.Author != nil {
		em.Author.Name = StrClamp(em.Author.Name, 256)
	}

	ch := ""
	if ca.ch != "" {
		ch = ca.ch
	} else {
		ch = ca.msg.ChannelID
	}

	nm, err := ca.sess.ChannelMessageSendEmbed(ch, em)
	if err != nil {
		err = fmt.Errorf("error sending embed in %s: %w", GetChannelName(ca.sess, ch), err)
		SendError(ca, err.Error())
	}
	return nm, err
}

// QuickEmbedTF sends a quick title, description, and footer embed
func QuickEmbedTF(ca CommandArgs, title string, content string, footer string) (*discordgo.Message, error) {
	em := &discordgo.MessageEmbed{Title: title, Description: content, Footer: &discordgo.MessageEmbedFooter{Text: footer}}
	return SendEmbed(ca, em)
}

// QuickEmbedT sends a quick title and description embed
func QuickEmbedT(ca CommandArgs, title string, content string) (*discordgo.Message, error) {
	em := &discordgo.MessageEmbed{Title: title, Description: content}
	return SendEmbed(ca, em)
}

// QuickEmbedF sends a quick description and footer embed
func QuickEmbedF(ca CommandArgs, content string, footer string) (*discordgo.Message, error) {
	em := &discordgo.MessageEmbed{Description: content, Footer: &discordgo.MessageEmbedFooter{Text: footer}}
	return SendEmbed(ca, em)
}

// QuickEmbed sends a quick description-only embed
func QuickEmbed(ca CommandArgs, content string) (*discordgo.Message, error) {
	em := &discordgo.MessageEmbed{Description: content}
	return SendEmbed(ca, em)
}

// SendError to a message's source channel with special error formatting
func SendError(ca CommandArgs, str string) {
	// not using SendEmbed here so we don't get stuck in a SendError loop
	ch := ""
	if ca.ch != "" {
		ch = ca.ch
	} else {
		ch = ca.msg.ChannelID
	}
	_, err := ca.sess.ChannelMessageSendEmbed(ch, &discordgo.MessageEmbed{Title: "error", Description: StrClamp(str, 2000), Color: 0xff0000})
	if err != nil {
		err = fmt.Errorf("error sending error in %s: %w", GetChannelName(ca.sess, ch), err)
		fmt.Println(err)
	}
}
