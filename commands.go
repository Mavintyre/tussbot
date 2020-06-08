package main

import (
	"fmt"
	"regexp"
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
//	- %P is replaced with first bot prefix
//	- ^ is replaced with ` so literals can be used for newlines
type Command struct {
	aliases   []string
	regexes   []string
	callback  func(CommandArgs) bool
	help      string
	emptyArg  bool
	hidden    bool
	adminOnly bool
	ownerOnly bool
}

// RegisterCommand to the bot
func RegisterCommand(cmd Command) {
	CommandList = append(CommandList, cmd)
}

// CommandArgs to be passed around easily
type CommandArgs struct {
	sess    *discordgo.Session
	msg     *discordgo.Message
	chO     string
	cmd     *Command
	alias   string
	args    string
	content string
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

	split := strings.SplitN(m.Content, " ", 2)
	mname := split[0]

	// prefixes are optional!
	if len(mname) > 1 {
		if Config.PrefixOptional {
			for _, p := range Config.Prefixes {
				if string(mname[0]) == p {
					mname = mname[1:]
					break
				}
			}
		}
	}

	margs := ""
	if len(split) > 1 {
		margs = split[1]
	}

	hasAdmin := HasAdmin(m.GuildID, m.Member)

	// run regex first in case it needs to consume
	for _, cmd := range CommandList {
		if cmd.ownerOnly && m.Author.ID != Config.OwnerID {
			continue
		}
		if cmd.adminOnly && !hasAdmin {
			continue
		}
		for _, r := range cmd.regexes {
			if regexp.MustCompile(r).MatchString(m.Content) {
				// no args and no alias
				shouldReturn := cmd.callback(CommandArgs{sess: s, msg: m, content: m.Content, cmd: &cmd})
				if shouldReturn {
					return
				}
			}
		}
	}

	for _, cmd := range CommandList {
		if cmd.ownerOnly && m.Author.ID != Config.OwnerID {
			continue
		}
		if cmd.adminOnly && !hasAdmin {
			continue
		}
		for _, a := range cmd.aliases {
			if a == mname {
				if margs == "" && !cmd.emptyArg {
					ShowHelp(CommandArgs{sess: s, msg: m}, cmd)
					return
				}
				cmd.callback(CommandArgs{sess: s, msg: m, args: margs, content: m.Content, alias: mname, cmd: &cmd})
				return
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
			footer += fmt.Sprintf("%s%s", Config.Prefixes[0], v)
			if i != len(cmd.aliases)-1 {
				footer += ", "
			}
		}
	}

	QuickEmbed(ca, QEmbed{
		title:   fmt.Sprintf("command help: %s%s", Config.Prefixes[0], cmd.aliases[0]),
		content: help,
		footer:  footer,
		colour:  helpColour,
	})
}

// HasAdmin returns true if member has the guild's admin role
func HasAdmin(gid string, mem *discordgo.Member) bool {
	if mem == nil {
		return false
	}

	grid := AdminRoleCache[gid]
	for _, mr := range mem.Roles {
		if mr == grid {
			return true
		}
	}
	return false
}

func init() {
	RegisterCommand(Command{
		aliases:  []string{"help"},
		hidden:   true,
		emptyArg: true,
		help:     ":egg:",
		callback: func(ca CommandArgs) bool {
			hasAdmin := HasAdmin(ca.msg.GuildID, ca.msg.Member)

			// show help for a command
			if ca.args != "" {
				for _, cmd := range CommandList {
					if cmd.ownerOnly && ca.msg.Author.ID != Config.OwnerID {
						continue
					}
					if cmd.adminOnly && !hasAdmin {
						continue
					}
					for _, a := range cmd.aliases {
						if a == ca.args {
							ShowHelp(ca, cmd)
							return false
						}
					}
				}
				SendError(ca, "command not found")
				return false
			}

			var list []string
			for _, cmd := range CommandList {
				if cmd.ownerOnly && ca.msg.Author.ID != Config.OwnerID {
					continue
				}
				if cmd.adminOnly && !hasAdmin {
					continue
				}
				if cmd.hidden {
					continue
				}
				help := formatTokens(cmd.help)
				firstline := strings.Split(help, "\n")[0]
				list = append(list, fmt.Sprintf("%s%s - %s", Config.Prefixes[0], cmd.aliases[0], firstline))
			}

			pfxText := ""
			if Config.PrefixOptional {
				pfxText = "command prefixes are optional!\n"
			}

			QuickEmbed(ca, QEmbed{
				title:   "bot commands",
				content: fmt.Sprintf("```%s```", strings.Join(list, "\n")),
				footer:  fmt.Sprintf("\"%shelp command\" for help with individual commands\n%sprefixes: %s", Config.Prefixes[0], pfxText, strings.Join(Config.Prefixes, " ")),
				colour:  helpColour,
			})

			return false
		}})
}
