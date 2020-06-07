package main

import (
	"errors"
	"fmt"
	"io"
	"time"
)

func init() {
	RegisterCommand(Command{
		aliases: []string{"play", "p"},
		help:    "play a song from url",
		callback: func(ca CommandArgs) {
			// parse url
			// get streamurl
			url := "soul.mp3"

			// join channel
			tc, err := ca.sess.State.Channel(ca.msg.ChannelID)
			if err != nil {
				return
			}

			g, err := ca.sess.State.Guild(tc.GuildID)
			if err != nil {
				return
			}

			for _, vs := range g.VoiceStates {
				if vs.UserID == ca.msg.Author.ID {
					// begin and save ref to ffmpeg session
					vc, err := ca.sess.ChannelVoiceJoin(g.ID, vs.ChannelID, false, true)
					if err != nil {
						return
					}

					done := make(chan error)
					ticker := time.NewTicker(time.Second)

					session := FFMPEGSession{}
					go session.Start(url, vc, done)

					for {
						select {
						case err := <-done:
							if err != nil && !errors.Is(err, io.EOF) {
								fmt.Println("ffmpeg session error:", err)
							}
							session.Stop()
							vc.Disconnect()
							return
						case <-ticker.C:
							pos := session.CurrentTime()
							fmt.Println(pos)
						}
					}
				}
			}
		},
	})
}
