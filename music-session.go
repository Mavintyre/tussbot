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
	queue     []queueSong
	playing   bool
	done      chan error
	ffmpeg    *FFMPEGSession
	voiceConn *discordgo.VoiceConnection
	voiceChan *discordgo.Channel
	sess      *discordgo.Session
	guild     string
	musicChan string
	embedID   string // TO DO: save this
	embedBM   *ButtonizedMessage
}

func (ms *musicSession) Play() {
	ms.Lock()
	defer ms.Unlock()

	song := ms.queue[0]
	ms.done = make(chan error, 10)
	go ms.ffmpeg.Start(song.url, 0, 1, ms.voiceConn, ms.voiceChan.Bitrate, ms.done)
	ms.playing = true
	ms.updateEmbed()
}

func (ms *musicSession) Pause() {
	ms.Lock()
	defer ms.Unlock()

	if ms.playing {
		ms.ffmpeg.SetPaused(!ms.ffmpeg.Paused())
	}
}

func (ms *musicSession) Skip() {
	ms.Lock()
	defer ms.Unlock()

	if ms.playing {
		ms.ffmpeg.Stop()
	}
}

func (ms *musicSession) Stop() {
	ms.Lock()
	defer ms.Unlock()

	if ms.playing {
		ms.queue = nil
		ms.ffmpeg.Stop()
	}

	if ms.voiceConn != nil && ms.voiceConn.Ready {
		ms.voiceConn.Disconnect()
	}
}

func (ms *musicSession) queueLoop() {
	ticker := time.NewTicker(time.Second * 10)
	for {
		select {
		case err := <-ms.done:
			if err != nil && !errors.Is(err, io.EOF) {
				SendErrorTemp(CommandArgs{sess: ms.sess, chO: ms.musicChan}, fmt.Sprintf("ffmpeg session error: %s", err), errorTimeout)
			}
			ms.ffmpeg.Cleanup()

			ms.Lock()
			if ms.queue == nil || len(ms.queue) < 1 {
				ms.playing = false
				ms.Unlock()
				return
			}

			ms.queue = append(ms.queue[:0], ms.queue[1:]...)
			newlen := len(ms.queue)
			ms.Unlock()

			if newlen > 0 {
				ms.Play()
			} else {
				ms.Lock()
				ms.playing = false
				ms.Unlock()
				// TO DO: disconnection timeout
				return
			}
		case <-ticker.C:
			ms.updateEmbed()
		}
	}
}

func (ms *musicSession) makeEmbed() *discordgo.MessageEdit {
	// TO DO: finish this

	me := &discordgo.MessageEdit{}
	//me.Content = queue
	em := &discordgo.MessageEmbed{}
	if len(ms.queue) > 0 {
		em.Title = "[length] song name"
		// link
		// image
		em.Description = "queued by dude"
		em.Footer = &discordgo.MessageEmbedFooter{Text: strconv.Itoa(int(ms.ffmpeg.CurrentTime().Seconds()))}
	} else {
		em.Title = "no song playing"
	}
	me.Embed = em
	// buttons
	return me
}

func (ms *musicSession) updateEmbed() {
	me := ms.makeEmbed()
	me.Channel = ms.embedBM.Msg.ChannelID
	me.ID = ms.embedBM.Msg.ID
	err := EditMessage(CommandArgs{sess: ms.sess, chO: ms.musicChan}, me)

	// stop playback if there is no embed
	if err != nil {
		ms.Stop()
	}
}

func (ms *musicSession) initEmbed() {
	ms.Lock()
	defer ms.Unlock()

	msg, err := ms.sess.ChannelMessage(ms.musicChan, ms.embedID)
	if err != nil {
		me := ms.makeEmbed()
		newmsg, err := SendEmbed(CommandArgs{sess: ms.sess, chO: ms.musicChan}, me.Embed)
		if err != nil {
			SendErrorTemp(CommandArgs{sess: ms.sess, chO: ms.musicChan}, fmt.Sprintf("couldn't create embed: %s", err), errorTimeout)
			return
		}
		msg = newmsg
		ms.embedID = msg.ID
		SetGuildMusicEmbed(ms.guild, msg.ID)
	}

	// destroy old embed if there is one
	if msg.ID != ms.embedID && ms.embedBM != nil {
		ms.embedBM.Close <- true
		ms.embedBM = nil
	}

	if ms.embedBM == nil {
		// TO DO: loop and replay
		bm := ButtonizeMessage(ms.sess, msg)
		go func() {
			bm.AddHandler("↪", func(bm *ButtonizedMessage) {
				//ms.Replay()
			})
			bm.AddHandler("⏹️", func(bm *ButtonizedMessage) {
				ms.Stop()
			})
			bm.AddHandler("⏯️", func(bm *ButtonizedMessage) {
				ms.Pause()
			})
			bm.AddHandler("⏭️", func(bm *ButtonizedMessage) {
				ms.Skip()
			})
			bm.AddHandler("🔄", func(bm *ButtonizedMessage) {
				//ms.Loop()
			})
		}()
		go bm.Listen()
		ms.embedBM = bm
	}
}
