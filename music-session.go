package main

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var embedUpdateFreq = 45
var timeoutSeconds = 10

// SongInfo stores data for one song in the queue
type SongInfo struct {
	URL       string
	Title     string
	Thumbnail string
	StreamURL string
	Duration  time.Duration
	QueuedBy  string
}

type musicSession struct {
	sync.Mutex
	queue     []*SongInfo
	playing   bool
	done      chan error
	ffmpeg    *FFMPEGSession
	voiceConn *discordgo.VoiceConnection
	voiceChan *discordgo.Channel
	sess      *discordgo.Session
	guild     string
	musicChan string
	embedID   string
	embedBM   *ButtonizedMessage
	looping   bool
	lastSong  *SongInfo
	startTO   time.Time
}

func (ms *musicSession) Play() {
	ms.Lock()
	defer ms.Unlock()

	song := ms.queue[0]
	ms.done = make(chan error, 10)
	go ms.ffmpeg.Start(song.StreamURL, 0, 1, ms.voiceConn, ms.voiceChan.Bitrate, ms.done)
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

	if ms.voiceConn != nil {
		ms.voiceConn.Disconnect()
	}
}

func (ms *musicSession) Loop() {
	ms.Lock()
	defer ms.Unlock()

	ms.looping = !ms.looping
}

func (ms *musicSession) Replay(caller *discordgo.Member) {
	if caller == nil {
		fmt.Println("no caller found for Replay")
		return
	}

	ms.Lock()
	song := ms.lastSong
	song.QueuedBy = caller.Nick
	playing := ms.playing
	ms.Unlock()

	if playing {
		ms.updateEmbed()
	} else {
		vs, vch, ok := getVoiceState(ms, ms.sess, ms.musicChan, caller.User.ID)
		if !ok {
			return
		}
		queueSong(ms, ms.sess, vs, vch, caller.User.ID, song)
	}
}

func (ms *musicSession) disconnectTimeout() {
	// record time this timeout started
	ms.Lock()
	thisStart := time.Now()
	ms.startTO = thisStart
	ms.Unlock()

	// sleep for timeout delay
	time.Sleep(time.Duration(timeoutSeconds) * time.Second)

	// get playing state and latest timeout started time
	ms.Lock()
	playing := ms.playing
	startTO := ms.startTO
	ms.Unlock()

	// if latest timeout was started at a differnt time
	// than this one, cancel this one as the other is newer
	if startTO != thisStart {
		return
	}

	// if not playing, stop and disconnect
	if playing {
		return
	}
	ms.Stop()
}

func (ms *musicSession) queueLoop() {
	ticker := time.NewTicker(time.Second * time.Duration(embedUpdateFreq))
	for {
		select {
		case err := <-ms.done:
			if err != nil && !errors.Is(err, io.EOF) {
				SendErrorTemp(CommandArgs{sess: ms.sess, chO: ms.musicChan}, fmt.Sprintf("ffmpeg session error: %s", err), errorTimeout)
			}
			ms.ffmpeg.Cleanup()

			ms.Lock()
			if len(ms.queue) > 0 {
				ms.lastSong = ms.queue[0]
			}

			if ms.queue == nil || len(ms.queue) < 1 {
				ms.playing = false
				ms.Unlock()
				ms.updateEmbed()
				return
			}

			if !ms.looping {
				ms.queue = append(ms.queue[:0], ms.queue[1:]...)
			}
			newlen := len(ms.queue)
			ms.Unlock()

			if newlen > 0 {
				ms.Play()
			} else {
				ms.Lock()
				ms.playing = false
				ms.Unlock()
				ms.updateEmbed()
				go ms.disconnectTimeout()
				return
			}
		case <-ticker.C:
			ms.updateEmbed()
		}
	}
}

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	return fmt.Sprintf("%02d:%02d", m, s)
}

func (ms *musicSession) makeEmbed() *discordgo.MessageEdit {
	me := &discordgo.MessageEdit{}

	queue := ""
	for i, v := range ms.queue {
		length := fmtDuration(v.Duration)
		queue += fmt.Sprintf("%02d.  **%s** [%s]  `%s`\n", i+1, v.Title, length, v.QueuedBy)
	}
	me.Content = &queue

	em := &discordgo.MessageEmbed{}
	if len(ms.queue) > 0 {
		s := ms.queue[0]
		length := fmtDuration(s.Duration)
		em.Title = fmt.Sprintf("%s [%s]", s.Title, length)
		em.URL = s.URL
		em.Image = &discordgo.MessageEmbedImage{URL: s.Thumbnail}
		em.Description = fmt.Sprintf("queued by `%s`", s.QueuedBy)
		looping := ""
		if ms.looping {
			looping = "\n(looping)"
		}
		em.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("current time: %s / %s\nupdates every %ds%s",
			fmtDuration(ms.ffmpeg.CurrentTime()), length, embedUpdateFreq, looping)}
	} else {
		em.Title = "no song playing"
		em.Description = "paste in a song link to begin"
	}
	me.Embed = em
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
		bm := ButtonizeMessage(ms.sess, msg)
		go func() {
			// TO DO: check if caller is in same voice channel before allowing anything
			bm.AddHandler("↪", func(bm *ButtonizedMessage, caller *discordgo.Member) {
				ms.Replay(caller)
			})
			bm.AddHandler("⏹️", func(bm *ButtonizedMessage, caller *discordgo.Member) {
				ms.Stop()
			})
			bm.AddHandler("⏯️", func(bm *ButtonizedMessage, caller *discordgo.Member) {
				ms.Pause()
			})
			bm.AddHandler("➡", func(bm *ButtonizedMessage, caller *discordgo.Member) {
				ms.Skip()
			})
			bm.AddHandler("🔄", func(bm *ButtonizedMessage, caller *discordgo.Member) {
				ms.Loop()
			})
		}()
		go bm.Listen()
		ms.embedBM = bm
	}
}
