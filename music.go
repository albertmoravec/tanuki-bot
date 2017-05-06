package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/youtube/v3"
	"log"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

type Player struct {
	IsPlaying       bool
	Queue           Queue
	SongChannel     chan *QueueItem
	PauseChannel    chan bool
	StopChannel     chan bool
	QuitChannel     chan bool
	VoiceConnection *discordgo.VoiceConnection
	Streamer        *dca.StreamingSession
	SendChannel     chan []int16
	ClientConfig    *jwt.Config
	GuildID         string
	DgoSession      *discordgo.Session
}

var (
	ErrPlayerConnected    error = errors.New("Player is already connected, use !stop")
	ErrPlayerNotConnected error = errors.New("Player is not connected, use !join")
)

func (cmds *Commands) InitPlayer() {
	//TODO make song recognition part of a service interface
	queueSong := CommandConstructor{
		Names:             []string{"queue", "q", "p"},
		Permission:        "queue",
		DefaultPermission: true,
		NoArguments:       false,
		MinArguments:      1,
		MaxArguments:      -1,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player == nil {
				return ErrPlayerNotConnected
			}

			youtubeRegexp := regexp.MustCompile(`youtu(?:be\.com/(?:v/|e(?:mbed)?/|watch\?v=)|\.be/)([\w-]{11}\b)`)

			for _, link := range raw {
				id := youtubeRegexp.FindStringSubmatch(link)

				if len(id) > 0 {
					yt, err := CreateQueueItem(id[1], m.Author.Username)
					if err != nil {
						log.Println(err)
						continue
					}

					bot.Player.Add(yt)
				} else {
					s.ChannelMessageSend(m.ChannelID, "No video matched")
				}
			}
			return nil
		},
	}

	queueList := CommandConstructor{
		Names:             []string{"queuelist", "qlist", "ql"},
		Permission:        "queueList",
		DefaultPermission: true,
		NoArguments:       false,
		MinArguments:      1,
		MaxArguments:      -1,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player == nil {
				return ErrPlayerNotConnected
			}

			if bot.Player.ClientConfig == nil {
				return errors.New("No Youtube API key provided")
			}

			listRegexp := regexp.MustCompile(`^.*(?:youtu.be/|list=)([^#&?]*).*\b`)

			service, err := youtube.New(bot.Player.ClientConfig.Client(context.Background()))
			if err != nil {
				return err
			}

			for _, link := range raw {
				id := listRegexp.FindStringSubmatch(link)

				if len(id) > 0 {
					items := make(chan *QueueItem)

					err := RetrievePlaylist(service, id[1], m.Author.Username, items)
					if err != nil {
						log.Println(err)
						continue
					}

					for item := range items {
						bot.Player.Add(item)
					}
				}
			}

			return nil
		},
	}

	skip := CommandConstructor{
		Names:             []string{"skip"},
		Permission:        "skip",
		DefaultPermission: true,
		NoArguments:       true,
		MinArguments:      0,
		MaxArguments:      -1,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player == nil {
				return ErrPlayerNotConnected
			}

			bot.Player.Skip()
			return nil
		},
	}

	stop := CommandConstructor{
		Names:             []string{"stop"},
		Permission:        "stop",
		DefaultPermission: true,
		NoArguments:       true,
		MinArguments:      0,
		MaxArguments:      -1,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player == nil {
				return ErrPlayerNotConnected
			}

			err := bot.Player.Stop()
			if err != nil {
				return err
			}

			bot.Player = nil

			return nil
		},
	}

	playlist := CommandConstructor{
		Names:             []string{"playlist", "list", "pls"},
		Permission:        "playlist",
		NoArguments:       true,
		DefaultPermission: true,
		MinArguments:      0,
		MaxArguments:      -1,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player == nil {
				return ErrPlayerNotConnected
			}

			var formatedList string

			queue, remaining, err := bot.Player.Queue.GetFirstN(10)
			if err != nil {
				return err
			}

			//TODO use embed(s)
			for pos, item := range queue {
				formatedList = strings.Join([]string{formatedList, strconv.Itoa(pos + 1), ". ", item.Info.Title, "\n"}, "")
			}
			if remaining > 0 {
				formatedList += fmt.Sprintf("+ %d more...", remaining)
			}
			s.ChannelMessageSend(m.ChannelID, formatedList)

			return nil
		},
	}

	move := CommandConstructor{
		Names:             []string{"move", "mov", "m"},
		Permission:        "move",
		DefaultPermission: true,
		NoArguments:       false,
		MinArguments:      1,
		MaxArguments:      2,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player == nil {
				return ErrPlayerNotConnected
			}

			var moveFrom, moveTo int

			moveFrom, err := strconv.Atoi(raw[0])
			if err != nil {
				return err
			}
			moveFrom-- // slices are 0-index, but appears as 1-indexed to the user

			if len(raw) == 2 {
				moveTo, err = strconv.Atoi(raw[1])
				if err != nil {
					return err
				}
			} else {
				moveTo = 2
			}
			moveTo-- // slices are 0-index, but appears as 1-indexed to the user

			return bot.Player.Queue.Move(moveFrom, moveTo)
		},
	}

	remove := CommandConstructor{
		Names:             []string{"remove", "rem", "r"},
		Permission:        "remove",
		DefaultPermission: true,
		NoArguments:       false,
		MinArguments:      1,
		MaxArguments:      1,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player == nil {
				return ErrPlayerNotConnected
			}

			i, err := strconv.Atoi(raw[0])
			if err != nil {
				return err
			}

			if i < 1 {
				return nil
			}

			if i == 1 {
				return errors.New("Cannot remove currently playing song")
			}

			i-- // slices are 0-index, but appears as 1-indexed to the user

			return bot.Player.Queue.Remove(i)
		},
	}

	join := CommandConstructor{
		Names:             []string{"join", "j"},
		Permission:        "join",
		DefaultPermission: true,
		NoArguments:       true,
		MinArguments:      0,
		MaxArguments:      -1,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player != nil {
				return ErrPlayerConnected
			}

			// for some reason I can't get voice states right after opening a session, so let's retrieve a guild here
			guild, err := bot.DiscordSession.Guild(bot.Config.Guild)
			if err != nil {
				log.Fatal(err)
			}

			for _, vState := range guild.VoiceStates {
				if vState.UserID == m.Author.ID {
					vc, err := s.ChannelVoiceJoin(vState.GuildID, vState.ChannelID, false, false)
					if err != nil {
						log.Println(err)
						return nil
					}

					bot.Player = CreatePlayer(bot.Config, s, vc)
				}
			}

			return nil
		},
	}

	info := CommandConstructor{
		Names:             []string{"info", "i"},
		Permission:        "info",
		DefaultPermission: true,
		NoArguments:       false,
		MinArguments:      0,
		MaxArguments:      1,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player == nil {
				return ErrPlayerNotConnected
			}

			var id int = 0
			if len(raw) > 0 {
				if raw[0] != "" {
					parsed, err := strconv.Atoi(raw[0])
					if err != nil {
						return err
					}

					id = parsed - 1
				}
			}

			song, err := bot.Player.Queue.Get(id)
			if err != nil {
				return err
			}

			embed := &discordgo.MessageEmbed{
				Fields: []*discordgo.MessageEmbedField{
					{
						Name:  "Title:",
						Value: song.Info.Title,
					},
					{
						Name:  "Link:",
						Value: song.Info.Link,
					},
					{
						Name:   "Requested by:",
						Value:  song.RequestedBy,
						Inline: true,
					},
					{
						Name:   "Length:",
						Value:  song.Info.Duration,
						Inline: true,
					},
				},
			}

			_, err = s.ChannelMessageSendEmbed(m.ChannelID, embed)

			return err
		},
	}

	pause := CommandConstructor{
		Names:             []string{"pause"},
		Permission:        "pause",
		DefaultPermission: true,
		NoArguments:       true,
		MinArguments:      0,
		MaxArguments:      -1,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player == nil {
				return ErrPlayerNotConnected
			}

			bot.Player.Pause()
			return nil
		},
	}

	purge := CommandConstructor{
		Names:             []string{"purge", "pur"},
		Permission:        "purge",
		DefaultPermission: true,
		NoArguments:       true,
		MinArguments:      0,
		MaxArguments:      -1,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player == nil {
				return ErrPlayerNotConnected
			}

			bot.Player.Purge()

			return nil
		},
	}

	find := CommandConstructor{
		Names:             []string{"find", "f"},
		Permission:        "find",
		DefaultPermission: true,
		NoArguments:       false,
		MinArguments:      1,
		MaxArguments:      -1,
		RunFunc: func(bot *Bot, raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if bot.Player == nil {
				return ErrPlayerNotConnected
			}

			if bot.Player.ClientConfig == nil {
				return errors.New("No Youtube API key provided")
			}

			service, err := youtube.New(bot.Player.ClientConfig.Client(context.Background()))
			if err != nil {
				return err
			}

			item, err := Find(service, strings.Join(raw, " "), m.Author.Username)
			if err != nil {
				log.Println(err)
			}

			bot.Player.Add(item)

			return nil
		},
	}

	cmds.RegisterCommands(&queueSong, &queueList, &skip, &stop, &playlist, &move, &remove, &info, &join, &pause, &purge, &find)
}

func CreatePlayer(config *Configuration, session *discordgo.Session, voice *discordgo.VoiceConnection) *Player {
	player := Player{
		Queue:           Queue{},
		SongChannel:     make(chan *QueueItem, 1),
		PauseChannel:    make(chan bool),
		StopChannel:     make(chan bool),
		QuitChannel:     make(chan bool),
		SendChannel:     make(chan []int16, 2),
		DgoSession:      session,
		VoiceConnection: voice,
	}

	if config.YoutubeAPIKey != "" {
		var err error
		player.ClientConfig, err = LoadYoutubeAPIConfig(config.YoutubeAPIKey)
		if err != nil {
			log.Println(err)
		}
	}

	go func() {
		for {
			select {
			case song := <-player.SongChannel:
				player.IsPlaying = true

				player.DgoSession.UpdateStatus(0, song.Info.Title)
				player.DgoSession.ChannelTopicEdit(config.TextChannel, "Playing: "+song.Info.Title)

				player.Play(song.Stream)

				player.DgoSession.UpdateStatus(0, "")
				player.DgoSession.ChannelTopicEdit(config.TextChannel, "")

				player.Queue.Remove(0)

				song, err := player.Queue.GetFirst()
				if err == nil {
					player.SongChannel <- song
				}
			case <-player.QuitChannel:
				player.IsPlaying = false
				return
			}
			player.IsPlaying = false
		}
	}()

	return &player
}

func (player *Player) Play(stream Playable) {
	encoder, err := dca.EncodeMem(stream.Play(), dca.StdEncodeOptions)
	defer encoder.Cleanup()
	if err != nil {
		log.Println(err)
		return
	}

	player.VoiceConnection.Speaking(true)
	defer player.VoiceConnection.Speaking(false)

	for player.VoiceConnection.Ready == false {
		runtime.Gosched()
	}

	done := make(chan error)
	player.Streamer = dca.NewStream(encoder, player.VoiceConnection, done)

	//wait for commands
	for {
		select {
		case <-player.StopChannel:
			stream.Stop()
			encoder.Stop()
			return
		case <-player.PauseChannel:
			if player.Streamer.Paused() {
				player.Streamer.SetPaused(false)
			} else {
				player.Streamer.SetPaused(true)
			}
		case err := <-done:
			if err != nil {
				log.Println(err)
			}
			return
		}
	}

}

func (player *Player) Purge() {
	player.Queue.Purge()

	if player.IsPlaying {
		player.StopChannel <- true
	}
}

func (player *Player) Stop() error {
	player.Purge()

	player.QuitChannel <- true

	if player.VoiceConnection != nil && player.Streamer != nil {
		// wait for the player to stop, not sure if this is necessary?
		for {
			if ok, _ := player.Streamer.Finished(); !ok { //Streamer not finished, do something else...
				runtime.Gosched()
			} else { //Streamer finished, disconnect...
				break
			}
		}
	}

	err := player.VoiceConnection.Disconnect()
	if err != nil {
		return err
	}

	return nil
}

func (player *Player) Add(item ...*QueueItem) {
	player.Queue.Add(item...)

	if !player.IsPlaying {
		song, err := player.Queue.GetFirst()
		if err == nil {
			player.SongChannel <- song
		}
	}
}

func (player *Player) Skip() {
	if player.IsPlaying {
		player.StopChannel <- true
	}
}

func (player *Player) Pause() {
	if player.IsPlaying {
		player.PauseChannel <- true
	}
}
