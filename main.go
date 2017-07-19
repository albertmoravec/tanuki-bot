package main

import (
	"flag"
	"github.com/bwmarrin/discordgo"
	"log"
	"os"
	"os/signal"
)

type Bot struct {
	Config         *Configuration
	Permissions    *PermissionsManager
	Commands       *Commands
	Player         *Player
	DiscordSession *discordgo.Session
}

var (
	Tanuki Bot
)

func init() {
	Tanuki.Config = &Configuration{}

	configPath := flag.String("c", "config.yml", "Config file path")
	flag.Parse()

	Tanuki.Config.Load(*configPath)
}

func (bot *Bot) Init() {
	if !bot.Config.Validate() {
		log.Fatal("Invalid configuration")
		return
	}

	bot.Commands = CreateCommands()

	bot.Permissions = bot.Commands.InitPermissions("permissions.json")
	bot.Commands.InitPlayer()

	var err error
	bot.DiscordSession, err = discordgo.New(bot.Config.Token)
	if err != nil {
		log.Fatal(err)
		return
	}

	bot.DiscordSession.AddHandler(bot.ProcessCommand)
	err = bot.DiscordSession.Open()
	if err != nil {
		log.Fatal(err)
		return
	}
}

func main() {
	Tanuki.Init()

	log.Println("Up and running!")

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	Tanuki.DiscordSession.Close()
}
