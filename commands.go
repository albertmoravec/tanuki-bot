package main

import (
	"github.com/bwmarrin/discordgo"
	"strings"
	"time"
	"reflect"
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

type Command struct {
	Name    string
	Command *CommandConstructor
}

type PermissionCommand struct {
	PermissionName string
	Command        *CommandConstructor
}

//TODO use map?
type CommandConstructors []*CommandConstructor
type Commands []Command
type PermissionsCommand []PermissionCommand

var (
	commands           Commands
	permissionsCommand PermissionsCommand
)

func (ccs CommandConstructors) Flatten() (cmds Commands, psc PermissionsCommand) {
	for _, c := range ccs {
		for _, cname := range c.Names {
			cmds = append(cmds, Command{Name: cname, Command: c})
		}
		psc = append(psc, PermissionCommand{PermissionName: c.Permission, Command: c})
	}
	return
}

func (cmds Commands) FindByName(name string) (cmd CommandConstructor) {
	for _, c := range cmds {
		if !strings.EqualFold(name, c.Name) {
			continue
		}
		cmd = *c.Command
	}
	return
}

func (psc PermissionsCommand) FindByPermission(name string) (cmd CommandConstructor) {
	for _, c := range psc {
		if !strings.EqualFold(name, c.PermissionName) {
			continue
		}
		cmd = *c.Command
	}
	return
}

func RegisterCommands(cmd ...*CommandConstructor) {
	cmds, psc := CommandConstructors(cmd).Flatten()
	commands = append(commands, cmds...)
	permissionsCommand = append(permissionsCommand, psc...)
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

	cmd := commands.FindByName(parsed[0])

	if reflect.DeepEqual(cmd, CommandConstructor{}) {
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
