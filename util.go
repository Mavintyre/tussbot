package main

import "github.com/bwmarrin/discordgo"

// Clamp an integer between two values
func Clamp(num int, min int, max int) int {
	if num > max {
		return max
	}
	if num < min {
		return min
	}
	return num
}

// StrClamp returns a string clamped to max length
func StrClamp(str string, max int) string {
	length := len(str)
	if length > max {
		return str[:max]
	}
	return str
}

// GetChannelName returns a channel's name or "<unknown channel>"
func GetChannelName(sess *discordgo.Session, id string) string {
	channel := "<unknown channel>"
	ch, err := sess.Channel(id)
	if err == nil {
		channel = ch.Name
	}
	return channel
}
