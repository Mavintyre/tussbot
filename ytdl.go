package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"time"
)

type ytdlFormat struct {
	URL      string
	Protocol string
	//Acodec   string
	Vcodec string
	ABR    int
}

type ytdlJSON struct {
	Title     string
	Thumbnail string
	Formats   []ytdlFormat
	Duration  float64
}

// ParseSeek string from string (ie "t=20s")
func ParseSeek(str string) int {
	regex := regexp.MustCompile(`(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s?)?`)
	if regex.MatchString(str) {
		// TO DO: is it necessary to check length on groups if MustCompile?
		groups := regex.FindAllStringSubmatch(str, -1)

		hours, minutes, seconds := 0, 0, 0

		strH, err := strconv.ParseInt(groups[0][1], 10, 64)
		if err == nil {
			hours = int(strH)
		}

		strM, err := strconv.ParseInt(groups[0][2], 10, 64)
		if err == nil {
			minutes = int(strM)
		}

		strS, err := strconv.ParseInt(groups[0][3], 10, 64)
		if err == nil {
			seconds = int(strS)
		}

		totalSeek := 0
		totalSeek += seconds
		totalSeek += minutes * 60
		totalSeek += hours * 60 * 60

		return totalSeek
	}
	return 0
}

// YTDL runs a youtube-dl child process and returns songInfo for a URL
//	Note: function is blocking
func YTDL(url string) (*SongInfo, error) {
	// remove list=... from youtube links
	// as currently this freezes the bot
	// TO DO: remove when implementing playlists
	re := regexp.MustCompile(`([&\?]list=[^&]+)`)
	if re.MatchString(url) {
		url = re.ReplaceAllString(url, "")
	}

	args := []string{
		url,
		"-J",
		"--user-agent", userAgent,
		"--referer", referer,
		"--geo-bypass",
		"--youtube-skip-dash-manifest",
		"-4", // force ipv4
	}

	stdout, err := exec.Command("youtube-dl", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("error starting youtube-dl process: %w", err)
	}

	var js ytdlJSON
	err = json.Unmarshal(stdout, &js)
	if err != nil {
		return nil, fmt.Errorf("error parsing youtube-dl json: %w", err)
	}

	if len(js.Formats) < 1 {
		return nil, errors.New("no streams found")
	}

	song := &SongInfo{}
	song.URL = url
	song.Title = js.Title
	song.Thumbnail = js.Thumbnail
	song.Duration = time.Duration(js.Duration) * time.Second

	// remove rtmp links (soundcloud)
	for i := len(js.Formats) - 1; i >= 0; i-- {
		fm := js.Formats[i]
		if fm.Protocol == "rtmp" {
			js.Formats = append(js.Formats[:i], js.Formats[i+1:]...)
		}
	}

	// sort by audio bitrate
	sort.Slice(js.Formats, func(i, j int) bool {
		return js.Formats[i].ABR > js.Formats[j].ABR
	})

	// attempt to find a high quality audio-only stream
	// 96 = discord's max ABR, so we want at least that
	for _, fm := range js.Formats {
		if fm.Vcodec == "none" && fm.ABR >= 96 {
			song.StreamURL = fm.URL
			break
		}
	}

	// didn't find any good audio-only streams
	// pick highest quality in the array
	if song.StreamURL == "" {
		song.StreamURL = js.Formats[0].URL
	}

	// parse &t=
	regex := regexp.MustCompile(`t=(.+)`)
	if regex.MatchString(url) {
		match := regex.FindAllStringSubmatch(url, -1)
		song.Seek = ParseSeek(match[0][1])
	}

	return song, nil
}
