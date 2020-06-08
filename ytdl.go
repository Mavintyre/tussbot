package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
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

func ytdl(url string) (*songInfo, error) {
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

	song := &songInfo{}
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

	// TO DO: parse &t=

	return song, nil
}
