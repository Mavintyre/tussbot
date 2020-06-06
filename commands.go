package main

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// CommandList of chat commands
var CommandList []Command

// Command definition and parameters
//	- set emptyArg to true if command accepts an empty argument
//		or else help will show when user calls with no arguments
//	- first line of help string is used as a short description
//	- %P is replaced with current bot prefix
//	- ^ is replaced with ` so literals can be used for newlines
type Command struct {
	aliases  []string
	callback func(CommandArgs)
	help     string
	emptyArg bool
	hidden   bool
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
				if margs == "" && !cmd.emptyArg {
					ShowHelp(CommandArgs{sess: s, msg: m}, cmd)
					return
				}
				cmd.callback(CommandArgs{sess: s, msg: m, args: margs, alias: mname, cmd: &cmd})
			}
		}
	}
}

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

// replaces special tokens in a string
//	%P = command prefix
//	^ = ` (so raw literals can be used for newlines)
//	fixes newline characters in string
func formatTokens(str string) string {
	str = strings.Replace(str, "%P", Config.Prefix, -1)
	str = strings.Replace(str, "^", "`", -1)
	str = strings.Replace(str, "\\n", "\n", -1)
	return str
}

// SendReply to a message's source channel with a string -- returns message and error
func SendReply(ca CommandArgs, str string) (*discordgo.Message, error) {
	str = formatTokens(str)
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
//	only title, description, field names & values, and footer text are run through formatTokens
func SendEmbed(ca CommandArgs, em *discordgo.MessageEmbed) (*discordgo.Message, error) {
	em.Title = StrClamp(formatTokens(em.Title), 256)
	em.Description = StrClamp(formatTokens(em.Description), 2048)

	for len(em.Fields) > 25 {
		em.Fields = em.Fields[:len(em.Fields)-1]
	}

	for _, field := range em.Fields {
		field.Name = StrClamp(formatTokens(field.Name), 256)
		field.Value = StrClamp(formatTokens(field.Value), 1024)
	}

	if em.Footer != nil {
		em.Footer.Text = StrClamp(formatTokens(em.Footer.Text), 2048)
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

// QEmbed provides a quick interface to create message embeds
//	quirk: colour can't be 0, if black is desired use 0x000001
type QEmbed struct {
	title   string
	content string
	footer  string
	colour  int
}

// QuickEmbed constructs a quick embed using QEmbed and sends it with SendEmbed
func QuickEmbed(ca CommandArgs, qem QEmbed) (*discordgo.Message, error) {
	em := &discordgo.MessageEmbed{Title: qem.title, Description: qem.content}
	if qem.footer != "" {
		em.Footer = &discordgo.MessageEmbedFooter{Text: qem.footer}
	}
	if qem.colour != 0 {
		em.Color = qem.colour
	}
	return SendEmbed(ca, em)
}

// SendError to a message's source channel in a premade error embed
func SendError(ca CommandArgs, str string) {
	str = formatTokens(str)

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

var helpColour = 0x00cc00

// ShowHelp posts a help embed for cmd
func ShowHelp(ca CommandArgs, cmd Command) {
	help := formatTokens(cmd.help)
	help = strings.Replace(help, "\t", "", -1)
	footer := ""
	if len(cmd.aliases) > 1 {
		footer = "other aliases: "
		for i, v := range cmd.aliases {
			if i == 0 {
				continue
			}
			footer += fmt.Sprintf("%s%s", Config.Prefix, v)
			if i != len(cmd.aliases)-1 {
				footer += ", "
			}
		}
	}

	QuickEmbed(ca, QEmbed{
		title:   fmt.Sprintf("command help: %s%s", Config.Prefix, cmd.aliases[0]),
		content: help,
		footer:  footer,
		colour:  helpColour,
	})
}

func init() {
	RegisterCommand(Command{
		aliases:  []string{"help"},
		hidden:   true,
		emptyArg: true,
		help:     ":egg:",
		callback: func(ca CommandArgs) {
			// show help for a command
			if ca.args != "" {
				for _, cmd := range CommandList {
					for _, a := range cmd.aliases {
						if a == ca.args {
							ShowHelp(ca, cmd)
							return
						}
					}
				}
				SendError(ca, "command not found")
				return
			}

			var list []string
			for _, cmd := range CommandList {
				if cmd.hidden {
					continue
				}
				help := formatTokens(cmd.help)
				firstline := strings.Split(help, "\n")[0]
				list = append(list, fmt.Sprintf("%s%s - %s", Config.Prefix, cmd.aliases[0], firstline))
			}

			QuickEmbed(ca, QEmbed{
				title:   "bot commands",
				content: fmt.Sprintf("```%s```", strings.Join(list, "\n")),
				footer:  fmt.Sprintf("\"%shelp command\" for help with individual commands", Config.Prefix),
				colour:  helpColour,
			})
		}})
}
