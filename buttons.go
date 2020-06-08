package main

import (
	"fmt"
	"sync"

	"github.com/bwmarrin/discordgo"
)

func nextReaction(sess *discordgo.Session) chan *discordgo.MessageReactionAdd {
	ch := make(chan *discordgo.MessageReactionAdd)
	sess.AddHandlerOnce(func(_ *discordgo.Session, ev *discordgo.MessageReactionAdd) {
		ch <- ev
	})
	return ch
}

// ButtonHandler is a callback function for when a button is pressed
type ButtonHandler func(*ButtonizedMessage, *discordgo.Member)

// ButtonizedMessage contains all info about a buttonized message
//	Listen must be called after all handlers are set up
//	send an int on the Close channel to stop listening
type ButtonizedMessage struct {
	sync.Mutex
	Msg      *discordgo.Message
	Sess     *discordgo.Session
	handlers map[string]ButtonHandler
	Close    chan bool
}

// Listen for reaction events
func (bm *ButtonizedMessage) Listen() {
	for {
		select {
		case ev := <-nextReaction(bm.Sess):
			if ev.UserID != bm.Sess.State.User.ID {
				if ev.MessageID == bm.Msg.ID {
					emoji := ev.Emoji.Name

					// will silently fail if bot doesn't have permissions
					bm.Sess.MessageReactionRemove(bm.Msg.ChannelID, bm.Msg.ID, emoji, ev.UserID)

					handler, ok := bm.handlers[emoji]
					if ok {
						mem, err := bm.Sess.GuildMember(ev.GuildID, ev.UserID)
						if err != nil {
							fmt.Println("couldn't get member for button event")
							handler(bm, nil)
						} else {
							handler(bm, mem)
						}
					}
				}
			}
		case <-bm.Close:
			return
		}
	}
}

// AddHandler for an emoji
func (bm *ButtonizedMessage) AddHandler(emoji string, handler ButtonHandler) {
	bm.Sess.MessageReactionAdd(bm.Msg.ChannelID, bm.Msg.ID, emoji)
	bm.Lock()
	bm.handlers[emoji] = handler
	bm.Unlock()
}

// ButtonizeMessage and return ButtonizedMessage
func ButtonizeMessage(sess *discordgo.Session, msg *discordgo.Message) *ButtonizedMessage {
	sess.MessageReactionsRemoveAll(msg.ChannelID, msg.ID)
	bm := &ButtonizedMessage{}
	bm.Msg = msg
	bm.Sess = sess
	bm.Close = make(chan bool, 10)
	bm.handlers = make(map[string]ButtonHandler)
	return bm
}
