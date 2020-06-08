package main

import (
	"sync"

	"github.com/bwmarrin/discordgo"
)

func NextReaction(sess *discordgo.Session) chan *discordgo.MessageReactionAdd {
	ch := make(chan *discordgo.MessageReactionAdd)
	sess.AddHandlerOnce(func(_ *discordgo.Session, ev *discordgo.MessageReactionAdd) {
		ch <- ev
	})
	return ch
}

type ButtonHandler func(*ButtonizedMessage)
type ButtonizedMessage struct {
	sync.Mutex
	msg      *discordgo.Message
	sess     *discordgo.Session
	handlers map[string]ButtonHandler
	Close    chan bool
}

func (bm *ButtonizedMessage) Listen() {
	for {
		select {
		case ev := <-NextReaction(bm.sess):
			if ev.UserID != bm.sess.State.User.ID {
				if ev.MessageID == bm.msg.ID {
					emoji := ev.Emoji.Name
					handler, ok := bm.handlers[emoji]
					if ok {
						handler(bm)
					}
					// will silently fail if bot doesn't have permissions
					bm.sess.MessageReactionRemove(bm.msg.ChannelID, bm.msg.ID, emoji, ev.UserID)
				}
			}
		case <-bm.Close:
			return
		}
	}
}

func (bm *ButtonizedMessage) Handle(emoji string, handler ButtonHandler) {
	bm.sess.MessageReactionAdd(bm.msg.ChannelID, bm.msg.ID, emoji)
	bm.Lock()
	bm.handlers[emoji] = handler
	bm.Unlock()
}

func ButtonizeMessage(sess *discordgo.Session, msg *discordgo.Message) *ButtonizedMessage {
	bm := &ButtonizedMessage{}
	bm.msg = msg
	bm.sess = sess
	bm.handlers = make(map[string]ButtonHandler)
	return bm
}
