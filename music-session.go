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
var timeoutSeconds = 60

// SongInfo stores data for one song in the queue
type SongInfo struct {
	URL       string
	Title     string
	Thumbnail string
	StreamURL string
	Duration  time.Duration
	QueuedBy  string
	Seek      int
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
	volume    float64
	seekStart int
	seekOv    int
	restart   bool
	paused    bool
}

func (ms *musicSession) Play() {
	ms.Lock()
	defer ms.Unlock()

	ms.done = make(chan error, 10)

	song := ms.queue[0]
	seek := song.Seek
	if ms.seekOv != 0 {
		seek = ms.seekOv
		ms.seekOv = 0
	}
	ms.seekStart = seek

	go ms.ffmpeg.Start(song.StreamURL, seek, ms.volume, ms.voiceConn, ms.voiceChan.Bitrate, ms.done)
	ms.playing = true
	ms.paused = false
	ms.updateEmbed()
}

func (ms *musicSession) Pause() {
	if ms.playing {
		ms.paused = !ms.paused
		ms.ffmpeg.SetPaused(!ms.ffmpeg.Paused())
		ms.updateEmbed()
	}
}

func (ms *musicSession) Skip() {
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

func (ms *musicSession) Loop() {
	ms.Lock()
	defer ms.Unlock()

	ms.looping = !ms.looping
	ms.updateEmbed()
}

func (ms *musicSession) Replay(caller *discordgo.Member) {
	if caller == nil {
		fmt.Println("no caller found for Replay")
		return
	}

	song := ms.lastSong
	song.QueuedBy = GetNick(caller)

	if ms.playing {
		ms.Lock()
		ms.queue = append(ms.queue, song)
		ms.Unlock()
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

	// if latest timeout was started at a differnt time
	// than this one, cancel this one as the other is newer
	if ms.startTO != thisStart {
		return
	}

	// if not playing, stop and disconnect
	if ms.playing {
		return
	}
	ms.Stop()
}

// Restart the current song at a desired seek position
// 	if seek == -1 then it will restart at its current time
func (ms *musicSession) Restart(seek int) {
	if !ms.playing {
		return
	}

	ms.Lock()
	if seek == -1 {
		ms.seekOv = int(ms.CurrentSeek().Seconds())
	} else {
		ms.seekOv = seek
	}
	ms.restart = true
	ms.Unlock()

	ms.Skip()
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

			if !ms.looping && !ms.restart {
				ms.queue = append(ms.queue[:0], ms.queue[1:]...)
			}
			ms.restart = false
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

		paused := ""
		if ms.paused {
			paused = "\n(paused)"
		}

		em.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("current time: %s / %s\nupdates every %ds\nvolume: %.2f%s%s",
			fmtDuration(ms.CurrentSeek()), length, embedUpdateFreq, ms.volume, looping, paused)}
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

func (ms *musicSession) allowButtons(uid string) bool {
	ch := ms.musicChan
	vch, _, err := getVoiceChannel(ms.sess, ch, uid)
	if err != nil {
		return false
	}

	if ms.playing && ms.voiceChan != nil && vch.ID != ms.voiceChan.ID {
		return false
	}

	return true
}

func (ms *musicSession) initEmbed() {
	ms.Lock()

	msg, err := ms.sess.ChannelMessage(ms.musicChan, ms.embedID)
	if err != nil {
		me := ms.makeEmbed()
		newmsg, err := SendEmbed(CommandArgs{sess: ms.sess, chO: ms.musicChan}, me.Embed)
		if err != nil {
			SendErrorTemp(CommandArgs{sess: ms.sess, chO: ms.musicChan}, fmt.Sprintf("couldn't create embed: %s", err), errorTimeout)
			ms.Unlock()
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

	if ms.embedBM != nil {
		ms.Unlock()
	} else {
		bm := ButtonizeMessage(ms.sess, msg)
		ms.embedBM = bm
		ms.Unlock()

		go func() {
			bm.AddHandler("↪", func(bm *ButtonizedMessage, caller *discordgo.Member) {
				if !ms.allowButtons(caller.User.ID) {
					return
				}
				ms.Replay(caller)
			})
			bm.AddHandler("⏹️", func(bm *ButtonizedMessage, caller *discordgo.Member) {
				if !ms.allowButtons(caller.User.ID) {
					return
				}
				ms.Stop()
			})
			bm.AddHandler("⏯️", func(bm *ButtonizedMessage, caller *discordgo.Member) {
				if !ms.allowButtons(caller.User.ID) {
					return
				}
				ms.Pause()
			})
			bm.AddHandler("➡", func(bm *ButtonizedMessage, caller *discordgo.Member) {
				if !ms.allowButtons(caller.User.ID) {
					return
				}
				ms.Skip()
			})
			bm.AddHandler("🔄", func(bm *ButtonizedMessage, caller *discordgo.Member) {
				if !ms.allowButtons(caller.User.ID) {
					return
				}
				ms.Loop()
			})
			bm.Listen()
		}()
	}
}

// CurrentSeek returns the current seeking time of the playing song
// combines encoder's returned time with session seek offset
func (ms *musicSession) CurrentSeek() time.Duration {
	if !ms.playing {
		return 0
	}

	seek := ms.seekStart
	encoderTime := int(ms.ffmpeg.CurrentTime().Seconds())
	return time.Duration(encoderTime+seek) * time.Second
}
