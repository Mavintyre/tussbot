package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

func init() {
	session := FFMPEGSession{}

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

					go session.Start(url, vc, done)

					for {
						select {
						case err := <-done:
							if err != nil && !errors.Is(err, io.EOF) {
								SendError(ca, fmt.Sprintf("ffmpeg session error:", err))
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

	RegisterCommand(Command{
		aliases:  []string{"volume", "vol", "v"},
		help:     "change or display playback volume",
		emptyArg: true,
		callback: func(ca CommandArgs) {
			if ca.args == "" {
				SendReply(ca, fmt.Sprintf("current playback volume: %.2f", session.Volume()))
				return
			}

			newVol, err := strconv.ParseFloat(ca.args, 64)
			if err != nil {
				SendError(ca, "error parsing volume: "+err.Error())
				return
			}

			session.SetVolume(newVol)
		}})
}
