package main

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// ClampI an integer between two values
func ClampI(num int, min int, max int) int {
	if num > max {
		return max
	}
	if num < min {
		return min
	}
	return num
}

// ClampF a float between two values
func ClampF(num float64, min float64, max float64) float64 {
	if num > max {
		return max
	}
	if num < min {
		return min
	}
	return num
}

// ClampStr returns a string clamped to max length
func ClampStr(str string, max int) string {
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

// GetRole resolves a role name to object
func GetRole(s *discordgo.Session, gid string, name string) (*discordgo.Role, error) {
	roles, err := s.GuildRoles(gid)
	if err != nil {
		return nil, fmt.Errorf("error getting role %s: %w", name, err)
	}
	roleName := strings.ToLower(name)
	for _, role := range roles {
		if strings.ToLower(role.Name) == roleName {
			return role, nil
		}
	}
	return nil, fmt.Errorf("role %s not found", name)
}

// GetNick from a member, or return username if no nickname set
func GetNick(mem *discordgo.Member) string {
	if mem.Nick != "" {
		return mem.Nick
	}
	return mem.User.Username
}
