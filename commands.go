package main

import (
	"github.com/bwmarrin/discordgo"
	"strings"
	"time"
)

type CommandConstructor struct {
	Names             []string
	Permission        string
	DefaultPermission bool
	NoArguments       bool
	MinArguments      int
	MaxArguments      int
	RunFunc           func(raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error
}

type NameCommand map[string]*CommandConstructor
type PermissionCommand map[string]*CommandConstructor

//TODO use map?
type CommandConstructors []*CommandConstructor

var (
	commands    NameCommand
	permissions PermissionCommand
)

func (ccs CommandConstructors) Flatten() {
	//TODO Init instead of this check
	if commands == nil {
		commands = make(NameCommand)
	}

	if permissions == nil {
		permissions = make(PermissionCommand)
	}

	for _, c := range ccs {
		for _, cname := range c.Names {
			commands[cname] = c
		}
		permissions[c.Permission] = c
	}
}

func RegisterCommands(cmd ...*CommandConstructor) {
	CommandConstructors(cmd).Flatten()
}

func ProcessCommand(s *discordgo.Session, m *discordgo.MessageCreate, perm *PermissionsManager) {
	var err error

	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		return
	}

	if m.ChannelID != Config.TextChannel && !channel.IsPrivate {
		return
	}

	parsed := strings.Split(strings.TrimPrefix(strings.TrimSpace(m.Content), "!"), " ")

	if len(parsed) <= 0 || !strings.HasPrefix(m.Content, "!") {
		return
	}

	cmd := commands[parsed[0]]

	if cmd == nil {
		s.ChannelMessageSend(m.ChannelID, "Command not found")
		return
	}

	if len(parsed[1:]) < cmd.MinArguments {
		s.ChannelMessageSend(m.ChannelID, "Not enough arguments provided")
		return
	}

	if cmd.MaxArguments != -1 && len(parsed[1:]) > cmd.MaxArguments {
		s.ChannelMessageSend(m.ChannelID, "Too many arguments provided")
		return
	}

	if !perm.Get(m.Author.ID, cmd.Permission, cmd.DefaultPermission) && m.Author.ID != Config.Owner {
		s.ChannelMessageSend(m.ChannelID, "Permission denied!")
		return
	}

	if cmd.NoArguments {
		err = cmd.RunFunc(nil, m, s)
	} else {
		err = cmd.RunFunc(parsed[1:], m, s)
	}

	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Error: "+err.Error())
	}

	go func() {
		time.Sleep(5 * time.Second)
		s.ChannelMessageDelete(Config.TextChannel, m.ID)
	}()
}
