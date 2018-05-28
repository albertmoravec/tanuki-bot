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
	RunFunc           func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error
}

type NameCommand map[string]*CommandConstructor
type PermissionCommand map[string]*CommandConstructor

type Commands struct {
	ByName       NameCommand
	ByPermission PermissionCommand
}

func CreateCommands() *Commands {
	return &Commands{
		ByName:       NameCommand{},
		ByPermission: PermissionCommand{},
	}
}

func (cmds *Commands) RegisterCommands(cmd ...*CommandConstructor) {
	for _, c := range cmd {
		for _, cmdName := range c.Names {
			cmds.ByName[cmdName] = c
		}
		cmds.ByPermission[c.Permission] = c
	}
}

func (bot *Bot) ProcessCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	var err error

	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		return
	}

	if m.ChannelID != bot.Config.TextChannel || channel.Type != discordgo.ChannelTypeGuildText {
		return
	}

	parsed := strings.Split(strings.TrimPrefix(strings.TrimSpace(m.Content), "!"), " ")

	if len(parsed) <= 0 || !strings.HasPrefix(m.Content, "!") {
		return
	}

	cmd := bot.Commands.ByName[parsed[0]]

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

	if !bot.Permissions.Get(m.Author.ID, cmd.Permission, cmd.DefaultPermission) && m.Author.ID != bot.Config.Owner {
		s.ChannelMessageSend(m.ChannelID, "Permission denied!")
		return
	}

	if cmd.NoArguments {
		err = cmd.RunFunc(bot, nil, m, s)
	} else {
		err = cmd.RunFunc(bot, parsed[1:], m, s)
	}

	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Error: "+err.Error())
	}

	go func() {
		time.Sleep(5 * time.Second)
		s.ChannelMessageDelete(bot.Config.TextChannel, m.ID)
	}()
}
