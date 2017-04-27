package main

import (
	"flag"
	"github.com/bwmarrin/discordgo"
	"log"
	"os"
	"os/signal"
)

var (
	Config config
)

func init() {
	Config = config{}

	//configPath := flag.String("c", "config.yml", "Config file path") // allow using -c flag, flags will override config file
	Config.Load("config.yml")

	flag.StringVar(&Config.Token, "a", Config.Token, "Auth token")
	flag.StringVar(&Config.Guild, "g", Config.Guild, "Guild ID")
	flag.StringVar(&Config.TextChannel, "t", Config.TextChannel, "Text channel ID")
	flag.StringVar(&Config.VoiceChannel, "v", Config.VoiceChannel, "Voice channel ID")
	flag.StringVar(&Config.Owner, "o", Config.Owner, "Owner ID")
	flag.StringVar(&Config.YoutubeAPIKey, "y", Config.YoutubeAPIKey, "Youtube API key")

	flag.StringVar(&Config.FFmpegPath, "ffmpeg", Config.FFmpegPath, "FFmpeg executable path")
	flag.Parse()
}

func main() {
	if !Config.Validate() {
		log.Fatal("Invalid configuration")
		return
	}

	perm := InitPermissions("permissions.json")

	d, err := discordgo.New(Config.Token)
	if err != nil {
		log.Fatal(err)
		return
	}

	d.AddHandler(handleCommand(perm))
	//d.LogLevel = discordgo.LogDebug

	err = d.Open()
	if err != nil {
		log.Fatal(err)
		return
	}

	InitPlayer(d, Config.Guild, Config.YoutubeAPIKey)

	log.Println("Up and running!")

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}

func handleCommand(perm *PermissionsManager) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		ProcessCommand(s, m, perm)
	}
}
