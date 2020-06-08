package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
)

func queueLoop(ffmpeg *FFMPEGSession, done chan error) {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, io.EOF) {
				SendError(ffmpeg.ca, fmt.Sprintf("ffmpeg session error: %s", err))
			}
			ffmpeg.Cleanup()
			// start next song in queue
			return
		case <-ticker.C:
			pos := ffmpeg.CurrentTime().Seconds()
			fmt.Println(pos)
		}
	}
}

func joinVoiceChannel(ca CommandArgs) (*discordgo.VoiceConnection, *discordgo.Channel, error) {
	tc, err := ca.sess.State.Channel(ca.msg.ChannelID)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't find text channel: %w", err)
	}

	g, err := ca.sess.State.Guild(tc.GuildID)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't find guild: %w", err)
	}

	for _, vs := range g.VoiceStates {
		if vs.UserID == ca.msg.Author.ID {
			vch, err := ca.sess.State.Channel(vs.ChannelID)
			if err != nil {
				return nil, nil, fmt.Errorf("couldn't find voice channel: %w", err)
			}

			// begin and save ref to ffmpeg session
			vc, err := ca.sess.ChannelVoiceJoin(g.ID, vs.ChannelID, false, true)
			if err != nil {
				return nil, nil, fmt.Errorf("couldn't join voice channel: %w", err)
			}

			return vc, vch, nil
		}
	}

	return nil, nil, errors.New("user not in a visible voice channel")
}

func init() {
	ffmpeg := FFMPEGSession{}
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
			vc, vch, err := joinVoiceChannel(ca)
			if err != nil {
				SendError(ca, fmt.Sprintf("%s", err))
				return
			}

			// start ffmpeg session
			go ffmpeg.Start(url, 0, 1, vc, vch.Bitrate, ca, done)
			go queueLoop(&ffmpeg, done)
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

			ffmpeg.SetPaused(newVol)
		}})

	RegisterCommand(Command{
		aliases: []string{"stop"},
		callback: func(ca CommandArgs) {
			ffmpeg.Stop()
		}})
}
