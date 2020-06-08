package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

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
	str = strings.Replace(str, "%P", Config.Prefixes[0], -1)
	str = strings.Replace(str, "^", "`", -1)
	str = strings.Replace(str, "\\n", "\n", -1)
	return str
}

// SendReply to a message's source channel with a string -- returns message and error
func SendReply(ca CommandArgs, str string) (*discordgo.Message, error) {
	str = formatTokens(str)
	str = StrClamp(str, 2000)

	ch := ""
	if ca.chO != "" {
		ch = ca.chO
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

func limitEmbedLength(em *discordgo.MessageEmbed) *discordgo.MessageEmbed {
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

	return em
}

// SendEmbed to a message's source channel with an embed
//	only title, description, field names & values, and footer text are run through formatTokens
func SendEmbed(ca CommandArgs, em *discordgo.MessageEmbed) (*discordgo.Message, error) {
	em = limitEmbedLength(em)

	ch := ""
	if ca.chO != "" {
		ch = ca.chO
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

// EditMessage edits a message while adhereing to string lengths
func EditMessage(ca CommandArgs, me *discordgo.MessageEdit) error {
	if me.Content != nil {
		content := *me.Content
		content = StrClamp(content, 2000)
		me.Content = &content
	}

	if me.Embed != nil {
		me.Embed = limitEmbedLength(me.Embed)
	}

	_, err := ca.sess.ChannelMessageEditComplex(me)
	if err != nil {
		err = fmt.Errorf("error editing message in %s: %w", GetChannelName(ca.sess, me.Channel), err)
		SendError(ca, err.Error())
	}
	return err
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
func SendError(ca CommandArgs, str string) *discordgo.Message {
	str = formatTokens(str)

	// not using SendEmbed here so we don't get stuck in a SendError loop
	ch := ""
	if ca.chO != "" {
		ch = ca.chO
	} else {
		ch = ca.msg.ChannelID
	}

	// TO DO: is there any instance where the author avatar will break this?
	var user *discordgo.User
	if ca.usrO != "" {
		u, err := ca.sess.User(ca.usrO)
		if err != nil {
			user = u
		}
	} else {
		user = ca.msg.Author
	}

	msg, err := ca.sess.ChannelMessageSendEmbed(ch, &discordgo.MessageEmbed{Description: StrClamp(str, 2000), Color: 0xff0000,
		Footer: &discordgo.MessageEmbedFooter{Text: ca.content}, Author: &discordgo.MessageEmbedAuthor{Name: "error", IconURL: user.AvatarURL("")}})
	if err != nil {
		err = fmt.Errorf("error sending error in %s: %w", GetChannelName(ca.sess, ch), err)
		fmt.Println(err)
		return nil
	}
	return msg
}

// SendErrorTemp sends an error and then deletes it after some timeout
func SendErrorTemp(ca CommandArgs, str string, timeout int) {
	msg := SendError(ca, str)
	if msg != nil {
		go func() {
			time.Sleep(time.Duration(timeout) * time.Second)
			ca.sess.ChannelMessageDelete(msg.ChannelID, msg.ID)
		}()
	}
}
