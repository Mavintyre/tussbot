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
	done := make(chan error)

	// TO DO: goroutine to wait for done and queue next

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
					vch, err := ca.sess.State.Channel(vs.ChannelID)
					if err != nil {
						return
					}

					// begin and save ref to ffmpeg session
					vc, err := ca.sess.ChannelVoiceJoin(g.ID, vs.ChannelID, false, true)
					if err != nil {
						return
					}

					ticker := time.NewTicker(time.Second)

					go session.Start(url, 0, 1, vc, vch.Bitrate, done)

					for {
						select {
						case err := <-done:
							if err != nil && !errors.Is(err, io.EOF) {
								SendError(ca, fmt.Sprintf("ffmpeg session error:", err))
							}
							session.Cleanup()
							vc.Disconnect()
							return
						case <-ticker.C:
							pos := session.CurrentTime().Seconds()
							fmt.Println(pos)
						}
					}
				}
			}

			SendError(ca, "not in a voice channel")
		},
	})

	RegisterCommand(Command{
		aliases: []string{"pause"},
		callback: func(ca CommandArgs) {
			newVol, err := strconv.ParseBool(ca.args)
			if err != nil {
				SendError(ca, "error parsing volume: "+err.Error())
				return
			}

			session.SetPaused(newVol)
		}})

	RegisterCommand(Command{
		aliases: []string{"stop"},
		callback: func(ca CommandArgs) {
			session.Stop()
		}})
}
