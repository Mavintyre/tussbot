package main

import "github.com/bwmarrin/discordgo"

// TO DO: what if role ID changes while bot is running?
var roleCache = make(map[string]map[string]string)

// CacheRole caches and returns a guild role by ID
func CacheRole(sess *discordgo.Session, gid string, roleName string) (bool, string) {
	roleID, ok := roleCache[gid][roleName]
	if ok {
		return true, roleID
	}

	role, err := GetRole(sess, gid, roleName)
	if err != nil {
		return false, ""
	}

	_, ok = roleCache[gid]
	if !ok {
		roleCache[gid] = make(map[string]string)
	}

	roleCache[gid][roleName] = role.ID
	return true, role.ID
}

var userCache = make(map[string]*discordgo.User)

// CacheUser caches and returns a user by ID
func CacheUser(sess *discordgo.Session, uid string) (bool, *discordgo.User) {
	user, ok := userCache[uid]
	if ok {
		return true, user
	}

	user, err := sess.User(uid)
	if err != nil {
		return false, nil
	}

	userCache[uid] = user
	return true, user
}
