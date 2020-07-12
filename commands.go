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
	roles     []string
	ownerOnly bool
	noDM      bool
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
	usrO    string
	cmd     *Command
	alias   string
	args    string
	content string
	isRegex bool
}

var roleCache = make(map[string]map[string]string)

// TO DO: what if role ID changes while bot is running?
func cacheRole(sess *discordgo.Session, gid string, roleName string) bool {
	_, ok := roleCache[gid][roleName]
	if ok {
		return true
	}

	role, err := GetRole(sess, gid, roleName)
	if err != nil {
		return false
	}

	_, ok = roleCache[gid]
	if !ok {
		roleCache[gid] = make(map[string]string)
	}

	roleCache[gid][roleName] = role.ID
	return true
}

// HasAccess checks if user has access to command
func HasAccess(sess *discordgo.Session, cmd Command, msg *discordgo.Message) bool {
	if cmd.ownerOnly && msg.Author.ID == Config.OwnerID {
		return true
	}
	if cmd.noDM && msg.Member == nil {
		return false
	}
	if len(cmd.roles) > 0 {
		if msg.Member == nil {
			return false
		}

		for _, roleName := range cmd.roles {
			if HasRole(sess, msg.Member, roleName) {
				return true
			}
		}
		return false
	}
	return true
}

// HandleCommand on message event
func HandleCommand(sess *discordgo.Session, m *discordgo.Message) {
	// fix discordgo bug
	if m.Member != nil && m.Member.User == nil {
		m.Member.User = m.Author
	}
	if m.Member != nil && m.Member.GuildID == "" {
		m.Member.GuildID = m.GuildID
	}

	defer func() {
		if r := recover(); r != nil {
			// get first 15 lines of stack
			buf := make([]byte, 1024)
			runtime.Stack(buf, false)
			str := string(buf)
			lines := strings.Split(str, "\n")
			stack := strings.Join(lines[:15], "\n")

			fmt.Println("<Recovered panic in HandleCommand>\n", stack)

			if Config.SendErrors {
				ch, err := GetDMChannel(sess, Config.OwnerID)
				if err != nil {
					fmt.Println("error DMing owner panic log", err)
					return
				}
				stack = strings.Replace(stack, "	", ">", -1)
				SendReply(CommandArgs{sess: sess, chO: ch.ID}, fmt.Sprintf("`<Recovered panic in HandleCommand>`\n```%s```", ClampStr(stack, 1957)))
			}
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

	// run regex first in case it needs to consume
	for _, cmd := range CommandList {
		for _, r := range cmd.regexes {
			if regexp.MustCompile(r).MatchString(m.Content) {
				if !HasAccess(sess, cmd, m) {
					continue
				}

				// no alias
				shouldReturn := cmd.callback(CommandArgs{isRegex: true, sess: sess, msg: m, args: margs, content: m.Content, alias: mname, cmd: &cmd})
				if shouldReturn {
					return
				}
			}
		}
	}

	for _, cmd := range CommandList {
		for _, a := range cmd.aliases {
			if a == mname {
				if !HasAccess(sess, cmd, m) {
					continue
				}

				if margs == "" && !cmd.emptyArg {
					ShowHelp(CommandArgs{sess: sess, msg: m}, cmd)
					return
				}
				cmd.callback(CommandArgs{sess: sess, msg: m, args: margs, content: m.Content, alias: mname, cmd: &cmd})
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

func init() {
	RegisterCommand(Command{
		aliases:  []string{"help"},
		hidden:   true,
		emptyArg: true,
		help:     ":egg:",
		callback: func(ca CommandArgs) bool {
			// show help for a command
			if ca.args != "" {
				for _, cmd := range CommandList {
					if !HasAccess(ca.sess, cmd, ca.msg) {
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
				if !HasAccess(ca.sess, cmd, ca.msg) {
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
