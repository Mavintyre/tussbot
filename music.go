package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type queueSong struct {
	url      string
	title    string
	queuedby string
	length   string
}

type musicSession struct {
	sync.Mutex
	queue   []queueSong
	playing bool
	done    chan error
	ffmpeg  *FFMPEGSession
	vc      *discordgo.VoiceConnection
	vch     *discordgo.Channel
	ca      CommandArgs
}

func (ms *musicSession) play() {
	ms.Lock()
	defer ms.Unlock()

	song := ms.queue[0]
	go ms.ffmpeg.Start(song.url, 0, 1, ms.vc, ms.vch.Bitrate, ms.done)
	ms.playing = true
}

func (ms *musicSession) queueLoop() {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case err := <-ms.done:
			if err != nil && !errors.Is(err, io.EOF) {
				SendError(ms.ca, fmt.Sprintf("ffmpeg session error: %s", err))
			}
			ms.ffmpeg.Cleanup()

			ms.Lock()
			if ms.queue == nil || len(ms.queue) < 1 {
				ms.Unlock()
				return
			}

			ms.queue = append(ms.queue[:0], ms.queue[1:]...)
			newlen := len(ms.queue)
			ms.Unlock()

			if newlen > 0 {
				ms.play()
			} else {
				ms.Lock()
				ms.playing = false
				ms.Unlock()
				// TO DO: disconnection timeout
				return
			}
		case <-ticker.C:
			// TO DO: update embed
			//pos := ms.ffmpeg.CurrentTime().Seconds()
			//fmt.Println(pos)
		}
	}
}

func getVoiceChannel(ca CommandArgs) (*discordgo.Channel, *discordgo.VoiceState, error) {
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

			return vch, vs, nil
		}
	}

	return nil, nil, errors.New("user not in a visible voice channel")
}

func joinVoiceChannel(ca CommandArgs, vs *discordgo.VoiceState) (*discordgo.VoiceConnection, error) {
	vc, err := ca.sess.ChannelVoiceJoin(vs.GuildID, vs.ChannelID, false, true)
	if err != nil {
		return nil, fmt.Errorf("couldn't join voice channel: %w", err)
	}
	return vc, nil
}

func init() {
	ms := musicSession{}
	ms.Lock()
	ms.ffmpeg = &FFMPEGSession{}
	ms.done = make(chan error)
	ms.Unlock()

	RegisterCommand(Command{
		aliases: []string{"play", "p"},
		help:    "play a song from url",
		callback: func(ca CommandArgs) {
			// parse url
			// get streamurl
			url := "soul.mp3"

			// check if user is in same channel
			vch, vs, err := getVoiceChannel(ca)
			if err != nil {
				SendError(ca, fmt.Sprintf("%s", err))
				return
			}

			ms.Lock()
			playing := ms.playing
			currentChan := ms.vch
			ms.Unlock()

			if playing && currentChan != nil && vch.ID != currentChan.ID {
				SendError(ca, "already playing in a different channel")
				return
			}

			// queue song
			s := queueSong{}
			s.url = url
			s.length = "1:23"
			s.queuedby = "dogu"
			s.title = "some song"

			ms.Lock()
			ms.queue = append(ms.queue, s)
			ms.Unlock()

			// TO DO: update embed

			if playing {
				fmt.Println("queued")
				// TO DO: delete ca.msg
				return
			}

			// join channel if not already playing
			vc, err := joinVoiceChannel(ca, vs)
			if err != nil {
				SendError(ca, fmt.Sprintf("%s", err))
				return
			}

			// start ffmpeg session
			ms.Lock()
			ms.vc = vc
			ms.vch = vch
			ms.ca = CommandArgs{sess: ca.sess, ch: ca.ch}
			ms.Unlock()

			ms.play()
			go ms.queueLoop()
			fmt.Println("playing")
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

			ms.ffmpeg.SetPaused(newVol)
		}})

	RegisterCommand(Command{
		aliases:  []string{"skip"},
		help:     "skips the current song in the queue",
		emptyArg: true,
		callback: func(ca CommandArgs) {
			ms.Lock()
			defer ms.Unlock()

			if ms.playing {
				ms.ffmpeg.Stop()
			}
		}})

	RegisterCommand(Command{
		aliases:  []string{"stop"},
		help:     "stops playing and leaves the channel",
		emptyArg: true,
		callback: func(ca CommandArgs) {
			ms.Lock()
			defer ms.Unlock()

			if ms.playing {
				ms.queue = nil

				ms.ffmpeg.Stop()
				ms.vc.Disconnect()
			}
		}})
}
