package main

import (
	"fmt"
	"runtime"
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
	aliases   []string
	callback  func(CommandArgs)
	help      string
	emptyArg  bool
	hidden    bool
	adminOnly bool
}

// RegisterCommand to the bot
func RegisterCommand(cmd Command) {
	CommandList = append(CommandList, cmd)
}

// CommandArgs to be passed around easily
type CommandArgs struct {
	sess  *discordgo.Session
	msg   *discordgo.Message
	chO   string
	cmd   *Command
	alias string
	args  string
}

// HandleCommand on message event
func HandleCommand(s *discordgo.Session, m *discordgo.Message) {
	defer func() {
		if r := recover(); r != nil {
			// get first 15 lines of stack
			buf := make([]byte, 1024)
			runtime.Stack(buf, false)
			str := string(buf)
			lines := strings.Split(str, "\n")
			stack := strings.Join(lines[:15], "\n")

			fmt.Println("<Recovered panic in HandleCommand>\n", stack)
			ch, err := GetDMChannel(s, Config.OwnerID)
			if err != nil {
				fmt.Println("error DMing owner panic log", err)
				return
			}
			stack = strings.Replace(stack, "	", ">", -1)
			SendReply(CommandArgs{sess: s, chO: ch.ID}, fmt.Sprintf("`<Recovered panic in HandleCommand>`\n```%s```", StrClamp(stack, 1957)))
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
			// TO DO: adminOnly filter
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
						// TO DO: adminOnly filter
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
				// TO DO: adminOnly filter
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
