package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"regexp"
	"sync"

	"github.com/bwmarrin/discordgo"
)

var errorTimeout = 5

var allowedLinks = []string{`youtube\.com\/watch\?v=.+`,
	`youtu\.be\/.+`,
	`soundcloud\.com\/.+\/.+`,
	`.+\.bandcamp\.com\/track\/.+`}

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
		SendError(ca, "guild has no music channel!\nget an admin to set one with %Pmusicchannel")
		return false
	}
	if ca.msg.ChannelID != chid {
		return false
	}
	return true
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
		help:    "play a song from url",
		callback: func(ca CommandArgs) bool {
			if !isMusicChannel(ca) {
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

			// check if user is in same channel
			vch, vs, err := getVoiceChannel(ca)
			if err != nil {
				SendErrorTemp(ca, fmt.Sprintf("%s", err), errorTimeout)
				return true
			}

			ms.Lock()
			playing := ms.playing
			currentChan := ms.voiceChan
			ms.Unlock()

			if playing && currentChan != nil && vch.ID != currentChan.ID {
				SendErrorTemp(ca, "already playing in a different channel", errorTimeout)
				return true
			}

			// parse url and queue song
			s, err := ytdl(ca.content)
			if err != nil {
				SendErrorTemp(ca, fmt.Sprintf("error querying song: %s", err), errorTimeout)
				return true
			}

			s.QueuedBy = ca.msg.Member.Nick

			ms.Lock()
			ms.queue = append(ms.queue, s)
			ms.Unlock()

			if playing {
				return true
			}

			// join channel if not already playing
			vc, err := joinVoiceChannel(ca, vs)
			if err != nil {
				SendErrorTemp(ca, fmt.Sprintf("%s", err), errorTimeout)
				return true
			}

			// start ffmpeg session
			ms.Lock()
			ms.voiceConn = vc
			ms.voiceChan = vch
			ms.Unlock()

			ms.Play()
			go ms.queueLoop()

			return true
		},
	})

	RegisterCommand(Command{
		aliases: []string{"musicchannel"},
		help: `marks this channel as the music channel\n
		bot will only listen to this channel for requests
		all music-related output will be in this channel
		use %Pmusicchannel again to recreate the embed`,
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
			getGuildSession(ca)

			// delete message afterwards
			ca.sess.ChannelMessageDelete(ca.msg.ChannelID, ca.msg.ID)

			return false
		}})
}
