package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
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
	embed   string
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

func (ms *musicSession) updateEmbed() {
	// TO DO: split in 2 funcs:
	//	- createEmbed - returns Embed{}
	//	- updateEmbed - updates current embed msg

	msg, err := ms.ca.sess.ChannelMessage(ms.ca.ch, ms.embed)
	if err != nil {
		SendError(ms.ca, fmt.Sprintf("error getting embed message: %s", err))
		return
	}

	me := &discordgo.MessageEdit{Channel: ms.ca.ch, ID: msg.ID}
	me.Embed = &discordgo.MessageEmbed{}
	ms.ca.sess.ChannelMessageEditComplex(me)
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

var listMutex sync.Mutex
var sessionList map[string]*musicSession

func getGuildSession(ca CommandArgs) *musicSession {
	listMutex.Lock()
	defer listMutex.Unlock()

	gid := ca.msg.GuildID
	ms, ok := sessionList[gid]
	if !ok {
		sessionList[gid] = &musicSession{}
		sessionList[gid].ffmpeg = &FFMPEGSession{}
		sessionList[gid].done = make(chan error)
		return sessionList[gid]
	}
	return ms
}

type musicSettings struct {
	MusicChannels map[string]string
}

var settingsCache musicSettings

func setGuildMusicChannel(gid string, cid string) {
	settingsCache.MusicChannels[gid] = cid
	b, err := json.Marshal(settingsCache)
	if err != nil {
		fmt.Println("Error marshaling JSON for music.json", err)
		return
	}
	err = ioutil.WriteFile("./settings/music.json", b, 0644)
	if err != nil {
		fmt.Println("Error saving music.json", err)
		return
	}
}

func isMusicChannel(ca CommandArgs) bool {
	chid, ok := settingsCache.MusicChannels[ca.msg.GuildID]
	if !ok {
		SendError(ca, "guild has no music channel!\nget an admin to set one with %Pmusicchannel")
		return false
	}
	if ca.msg.ChannelID != chid {
		return false
	}
	return true
}

func init() {
	// TO DO: wrapper for SendError that deletes after x seconds

	// initialize session list
	listMutex = sync.Mutex{}
	sessionList = make(map[string]*musicSession)

	// load music settings
	settingsjson, err := ioutil.ReadFile("./settings/music.json")
	if err == nil {
		err = json.Unmarshal(settingsjson, &settingsCache)
		if err != nil {
			fmt.Println("JSON error in music.json", err)
		}
	} else {
		fmt.Println("Unable to read music.json, using empty")
		settingsCache.MusicChannels = make(map[string]string)
	}

	// register commands
	RegisterCommand(Command{
		aliases: []string{"play", "p"},
		help:    "play a song from url",
		callback: func(ca CommandArgs) {
			if !isMusicChannel(ca) {
				return
			}
			ms := getGuildSession(ca)

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
		aliases:  []string{"pause"},
		help:     "pauses or resumes music",
		emptyArg: true,
		callback: func(ca CommandArgs) {
			if !isMusicChannel(ca) {
				return
			}
			ms := getGuildSession(ca)

			// TO DO: replace setPaused?
			ms.ffmpeg.SetPaused(!ms.ffmpeg.Paused())
		}})

	RegisterCommand(Command{
		aliases:  []string{"skip"},
		help:     "skips the current song in the queue",
		emptyArg: true,
		callback: func(ca CommandArgs) {
			if !isMusicChannel(ca) {
				return
			}
			ms := getGuildSession(ca)

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
			if !isMusicChannel(ca) {
				return
			}
			ms := getGuildSession(ca)

			ms.Lock()
			defer ms.Unlock()

			if ms.playing {
				ms.queue = nil

				ms.ffmpeg.Stop()
				ms.vc.Disconnect()
			}
		}})

	RegisterCommand(Command{
		aliases: []string{"musicchannel"},
		help: `marks this channel as the music channel\n
		bot will only listen to this channel for requests
		all music-related output will be in this channel
		use %Pmusicchannel again to recreate the embed`,
		emptyArg:  true,
		adminOnly: true,
		callback: func(ca CommandArgs) {
			setGuildMusicChannel(ca.msg.GuildID, ca.msg.ChannelID)
			// TO DO: create initial (empty) embed
		}})
}
