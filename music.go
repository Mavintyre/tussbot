package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
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

func (ms *musicSession) play() {
	ms.Lock()
	defer ms.Unlock()

	song := ms.queue[0]
	ms.done = make(chan error, 10)
	go ms.ffmpeg.Start(song.url, 0, 1, ms.voiceConn, ms.voiceChan.Bitrate, ms.done)
	ms.playing = true
}

func (ms *musicSession) stop() {
	ms.Lock()
	defer ms.Unlock()

	ms.queue = nil
	ms.ffmpeg.Stop()
	ms.voiceConn.Disconnect()
}

func (ms *musicSession) queueLoop() {
	ticker := time.NewTicker(time.Second * 10)
	for {
		select {
		case err := <-ms.done:
			if err != nil && !errors.Is(err, io.EOF) {
				SendError(CommandArgs{sess: ms.sess, chO: ms.musicChan}, fmt.Sprintf("ffmpeg session error: %s", err))
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
				ms.play()
			} else {
				ms.Lock()
				ms.playing = false
				ms.Unlock()
				// TO DO: disconnection timeout
				return
			}
		case <-ticker.C:
			// update embed
			ms.updateEmbed()
		}
	}
}

func (ms *musicSession) makeEmbed() *discordgo.MessageEdit {
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
	me.Channel = ms.embedBM.msg.ChannelID
	me.ID = ms.embedBM.msg.ID
	err := EditMessage(CommandArgs{sess: ms.sess, chO: ms.musicChan}, me)
	if err != nil {
		ms.stop()
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
			SendError(CommandArgs{sess: ms.sess, chO: ms.musicChan}, fmt.Sprintf("couldn't create embed: %s", err))
			return
		}
		msg = newmsg
		ms.embedID = msg.ID
		setGuildMusicEmbed(ms.guild, msg.ID)
	}

	if msg.ID != ms.embedID && ms.embedBM != nil {
		ms.embedBM.Close <- true
		ms.embedBM = nil
	}

	if ms.embedBM == nil {
		bm := ButtonizeMessage(ms.sess, msg)
		bm.Handle("💯", func(bm *ButtonizedMessage) {
			fmt.Println("caught 💯")
		})
		go bm.Listen()
		ms.embedBM = bm
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
		sessionList[gid].guild = gid
		sessionList[gid].sess = ca.sess
		chid, ok := settingsCache.MusicChannels[gid]
		if ok {
			sessionList[gid].musicChan = chid
		}
		emid, ok := settingsCache.MusicEmbeds[gid]
		if ok {
			sessionList[gid].embedID = emid
			sessionList[gid].initEmbed()
		}
		return sessionList[gid]
	}

	ms.initEmbed()
	return ms
}

type musicSettings struct {
	MusicChannels map[string]string
	MusicEmbeds   map[string]string
}

var settingsCache musicSettings

func saveSettings() {
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

func setGuildMusicEmbed(gid string, mid string) {
	settingsCache.MusicEmbeds[gid] = mid
	saveSettings()
}

func setGuildMusicChannel(gid string, cid string) {
	settingsCache.MusicChannels[gid] = cid
	saveSettings()
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
	}

	// initialilze empty
	if settingsCache.MusicChannels == nil {
		settingsCache.MusicChannels = make(map[string]string)
	}
	if settingsCache.MusicEmbeds == nil {
		settingsCache.MusicEmbeds = make(map[string]string)
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
			currentChan := ms.voiceChan
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
			ms.voiceConn = vc
			ms.voiceChan = vch
			ms.Unlock()

			ms.play()
			go ms.queueLoop()
			fmt.Println("playing")

			// TO DO: delete msg
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
			// TO DO: delete msg
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
			// TO DO: delete msg
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
			playing := ms.playing
			ms.Unlock()

			if playing {
				ms.stop()
			}

			// TO DO: delete msg
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
			oldem, ok := settingsCache.MusicEmbeds[ca.msg.GuildID]

			setGuildMusicChannel(ca.msg.GuildID, ca.msg.ChannelID)

			if ok {
				ca.sess.ChannelMessageDelete(ca.msg.ChannelID, oldem)
			}

			ms := getGuildSession(ca)
			ms.initEmbed()

			// TO DO: delete msg
		}})
}
