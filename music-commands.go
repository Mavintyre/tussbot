package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

var errorTimeout = 5

var allowedLinks = []string{`youtube\.com\/watch\?v=.+`,
	`youtu\.be\/.+`,
	`soundcloud\.com\/.+\/.+`,
	`.+\.bandcamp\.com\/track\/.+`}

func getVoiceChannel(sess *discordgo.Session, ch string, uid string) (*discordgo.Channel, *discordgo.VoiceState, error) {
	tc, err := sess.State.Channel(ch)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't find text channel: %w", err)
	}

	g, err := sess.State.Guild(tc.GuildID)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't find guild: %w", err)
	}

	for _, vs := range g.VoiceStates {
		if vs.UserID == uid {
			vch, err := sess.State.Channel(vs.ChannelID)
			if err != nil {
				return nil, nil, fmt.Errorf("couldn't find voice channel: %w", err)
			}

			return vch, vs, nil
		}
	}

	return nil, nil, errors.New("user not in a visible voice channel")
}

func joinVoiceChannel(sess *discordgo.Session, vs *discordgo.VoiceState) (*discordgo.VoiceConnection, error) {
	vc, err := sess.ChannelVoiceJoin(vs.GuildID, vs.ChannelID, false, true)
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

func saveMusicSettings() {
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

// SetGuildMusicEmbed sets the guild's music embed ID and saves it
func SetGuildMusicEmbed(gid string, mid string) {
	settingsCache.MusicEmbeds[gid] = mid
	saveMusicSettings()
}

func setGuildMusicChannel(gid string, cid string) {
	settingsCache.MusicChannels[gid] = cid
	saveMusicSettings()
}

func isMusicChannel(ca CommandArgs) bool {
	chid, ok := settingsCache.MusicChannels[ca.msg.GuildID]
	if !ok {
		return false
	}
	if ca.msg.ChannelID != chid {
		return false
	}
	return true
}

func getVoiceState(ms *musicSession, sess *discordgo.Session, ch string, uid string) (*discordgo.VoiceState, *discordgo.Channel, bool) {
	ca := CommandArgs{sess: sess, chO: ch, usrO: uid}

	// check if user is in same channel
	vch, vs, err := getVoiceChannel(sess, ch, uid)
	if err != nil {
		SendErrorTemp(ca, fmt.Sprintf("%s", err), errorTimeout)
		return nil, nil, false
	}

	ms.Lock()
	playing := ms.playing
	currentChan := ms.voiceChan
	ms.Unlock()

	if playing && currentChan != nil && vch.ID != currentChan.ID {
		SendErrorTemp(ca, "already playing in a different channel", errorTimeout)
		return nil, nil, false
	}

	return vs, vch, true
}

func queueSong(ms *musicSession, sess *discordgo.Session, vs *discordgo.VoiceState, vch *discordgo.Channel, uid string, song *SongInfo) {
	ca := CommandArgs{sess: sess, chO: vch.ID, usrO: uid}

	ms.Lock()
	ms.queue = append(ms.queue, song)
	playing := ms.playing
	ms.Unlock()

	if playing {
		ms.updateEmbed()
		return
	}

	ms.Lock()
	ms.playing = true
	ms.Unlock()

	// join channel
	vc, err := joinVoiceChannel(sess, vs)
	if err != nil {
		ms.Lock()
		ms.playing = true
		ms.Unlock()
		SendErrorTemp(ca, fmt.Sprintf("%s", err), errorTimeout)
		return
	}

	// start ffmpeg session
	ms.Lock()
	ms.voiceConn = vc
	ms.voiceChan = vch
	ms.volume = 1
	ms.Unlock()

	ms.Play()
	go ms.queueLoop()
}

func init() {
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
		regexes: []string{`[\s\S]+`},
		help: `play a song from url\n
		command is optional: you can just paste in a URL\n
		^%Pplay https://www.youtube.com/watch?v=asdf123^
		^https://www.youtube.com/watch?v=asdf123^`,
		callback: func(ca CommandArgs) bool {
			if !isMusicChannel(ca) {
				return false
			}

			// allowed commands in music channel
			// TO DO: some kind of prefix to allow admin role to bypass?
			if strings.Contains(ca.content, "volume") || strings.Contains(ca.content, "vol") ||
				strings.Contains(ca.content, "seek") ||
				strings.Contains(ca.content, "setmusic") {
				return false
			}

			ca.sess.ChannelMessageDelete(ca.msg.ChannelID, ca.msg.ID)

			found := false
			for _, r := range allowedLinks {
				if regexp.MustCompile(r).MatchString(ca.content) {
					found = true
					break
				}
			}

			if !found {
				SendErrorTemp(ca, "not an allowed link", errorTimeout)
				return true
			}

			ms := getGuildSession(ca)

			vs, vch, ok := getVoiceState(ms, ca.sess, ca.msg.ChannelID, ca.msg.Author.ID)
			if !ok {
				return true
			}

			// parse url and queue song
			// note: ytdl is blocking!
			song, err := YTDL(ca.content)
			if err != nil {
				SendErrorTemp(ca, fmt.Sprintf("error querying song: %s", err), errorTimeout)
				return true
			}

			song.QueuedBy = GetNick(ca.msg.Member)
			queueSong(ms, ca.sess, vs, vch, ca.msg.Author.ID, song)

			return true
		},
	})

	RegisterCommand(Command{
		aliases: []string{"setmusic"},
		help: `marks this as the music channel\n
		bot will only listen to this channel for requests
		all music-related output will be in this channel
		use %setmusic again to recreate the embed`,
		emptyArg:  true,
		adminOnly: true,
		callback: func(ca CommandArgs) bool {
			// keep note of old embed
			oldem, ok := settingsCache.MusicEmbeds[ca.msg.GuildID]

			// set channel setting
			setGuildMusicChannel(ca.msg.GuildID, ca.msg.ChannelID)

			// if there is an old embed, delete it
			if ok {
				ca.sess.ChannelMessageDelete(ca.msg.ChannelID, oldem)
			}

			// call getGuildSession to reinitialize embed
			// TO DO: set ms.musicChan manually
			// if ms is already created, musicChan is never set
			// to the new channel!
			getGuildSession(ca)

			// delete message afterwards
			ca.sess.ChannelMessageDelete(ca.msg.ChannelID, ca.msg.ID)

			return false
		}})

	RegisterCommand(Command{
		aliases: []string{"volume", "vol"},
		help: `change volume\n
		^%Pvolume 0.5^`,
		callback: func(ca CommandArgs) bool {
			if !isMusicChannel(ca) {
				return false
			}
			ca.sess.ChannelMessageDelete(ca.msg.ChannelID, ca.msg.ID)

			vol, err := strconv.ParseFloat(ca.args, 64)
			if err != nil {
				SendErrorTemp(ca, fmt.Sprintf("couldn't parse volume: %s", err), errorTimeout)
				return true
			}
			vol = ClampF(vol, 0.1, 1.5)

			ms := getGuildSession(ca)

			// TO DO: cleaner volume func on musicSession
			ms.Lock()
			ms.volume = vol
			ms.Unlock()

			ms.Restart(-1)
			return true
		}})

	RegisterCommand(Command{
		aliases: []string{"seek"},
		help: `seek some time into the current song\n
			^%Pseek 30^`,
		callback: func(ca CommandArgs) bool {
			if !isMusicChannel(ca) {
				return false
			}
			ca.sess.ChannelMessageDelete(ca.msg.ChannelID, ca.msg.ID)

			seek := ParseSeek(ca.args)

			ms := getGuildSession(ca)
			ms.Restart(seek)
			return true
		}})
}
