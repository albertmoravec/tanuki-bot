package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/rylio/ytdl"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/youtube/v3"
	"log"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"github.com/jonas747/dca"
)

type Player struct {
	IsPlaying       bool
	Queue           Queue
	QueueChannel    chan QueueItem
	SongChannel     chan QueueItem
	NextChannel     chan bool
	StopChannel     chan bool
	VoiceConnection *discordgo.VoiceConnection
	FFmpeg          *exec.Cmd
	SendChannel     chan []int16
	DgoSession      *discordgo.Session
	ClientConfig    *jwt.Config
}

const (
	channels  int = 2     // 1 for mono, 2 for stereo
	frameRate int = 48000 // audio sampling rate
)

var (
	ErrPlayerConnected    error = errors.New("Player is already connected, use !stop")
	ErrPlayerNotConnected error = errors.New("Player is not connected, use !join")
)

func InitPlayer(s *discordgo.Session, gID string, ytApiKeyPath string) {
	player := Player{
		Queue:        Queue{},
		SongChannel:  make(chan QueueItem),
		QueueChannel: make(chan QueueItem),
		NextChannel:  make(chan bool),
		StopChannel:  make(chan bool),
		SendChannel:  make(chan []int16, 2),
		DgoSession:   s,
	}

	if ytApiKeyPath != "" {
		var err error
		player.ClientConfig, err = LoadYoutubeAPIConfig(ytApiKeyPath)
		if err != nil {
			log.Println(err)
		}
	}

	//d.VoiceConnections[guild].LogLevel = discordgo.LogDebug

	//TODO make song recognition part of a service interface
	queueSong := CommandConstructor{
		Names:             []string{"queue", "q", "p"},
		Permission:        "queue",
		DefaultPermission: true,
		NoArguments:       false,
		MinArguments:      1,
		MaxArguments:      -1,
		RunFunc: func(raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if player.VoiceConnection == nil {
				return ErrPlayerNotConnected
			}
			youtubeRegexp := regexp.MustCompile(`youtu(?:be\.com/(?:v/|e(?:mbed)?/|watch\?v=)|\.be/)([\w-]{11}\b)`)

			for _, link := range raw {
				id := youtubeRegexp.FindStringSubmatch(link)

				if len(id) > 0 {
					video, err := ytdl.GetVideoInfo(id[1])
					if err != nil {
						log.Print(err)
					}

					stream := YoutubeItem{video, nil}

					player.QueueChannel <- QueueItem{
						Stream:      &stream,
						Info:        stream.GetInfo(),
						RequestedBy: m.Author.Username,
					}
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
		RunFunc: func(raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if player.VoiceConnection == nil {
				return ErrPlayerNotConnected
			}

			if player.ClientConfig == nil {
				return errors.New("No Youtube API key provided")
			}

			listRegexp := regexp.MustCompile(`youtube\.com/(?:playlist\?list=|watch\?v=[\w-]{11}(?:&index=\d+)*&list=)([\w-]{34}\b)`)

			service, err := youtube.New(player.ClientConfig.Client(context.Background()))
			if err != nil {
				return err
			}

			for _, link := range raw {
				id := listRegexp.FindStringSubmatch(link)

				if len(id) > 0 {
					playlistItems, err := service.PlaylistItems.List("snippet").PlaylistId(id[1]).MaxResults(50).Do()
					if err != nil {
						return err
					}

					for _, video := range playlistItems.Items {
						video, err := ytdl.GetVideoInfo(video.Snippet.ResourceId.VideoId)
						if err != nil {
							log.Println(err)
							continue
						}

						stream := YoutubeItem{video, nil}

						player.QueueChannel <- QueueItem{
							Stream:      &stream,
							Info:        stream.GetInfo(),
							RequestedBy: m.Author.Username,
						}
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
		RunFunc: func(_ []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if player.IsPlaying {
				player.StopChannel <- true
			}
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
		RunFunc: func(_ []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if player.VoiceConnection == nil {
				return ErrPlayerNotConnected
			}

			return player.Stop()
		},
	}

	playlist := CommandConstructor{
		Names:             []string{"playlist", "list", "pls"},
		Permission:        "playlist",
		NoArguments:       true,
		DefaultPermission: true,
		MinArguments:      0,
		MaxArguments:      -1,
		RunFunc: func(_ []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			var formatedList string

			queue, remaining, err := player.Queue.GetFirstN(10)
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
		RunFunc: func(positions []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			var moveFrom, moveTo int

			moveFrom, err := strconv.Atoi(positions[0])
			if err != nil {
				return err
			}
			moveFrom-- // slices are 0-index, but appears as 1-indexed to the user

			if len(positions) == 2 {
				moveTo, err = strconv.Atoi(positions[1])
				if err != nil {
					return err
				}
			} else {
				moveTo = 2
			}
			moveTo-- // slices are 0-index, but appears as 1-indexed to the user

			return player.Queue.Move(moveFrom, moveTo)
		},
	}

	remove := CommandConstructor{
		Names:             []string{"remove", "rem", "r"},
		Permission:        "remove",
		DefaultPermission: true,
		NoArguments:       false,
		MinArguments:      1,
		MaxArguments:      1,
		RunFunc: func(num []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			i, err := strconv.Atoi(num[0])
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

			return player.Queue.Remove(i)
		},
	}

	//TODO implement proper join (join more channels maybe? would probably require dca)
	join := CommandConstructor{
		Names:             []string{"join", "j"},
		Permission:        "join",
		DefaultPermission: true,
		NoArguments:       true,
		MinArguments:      0,
		MaxArguments:      -1,
		RunFunc: func(raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if player.VoiceConnection != nil {
				return ErrPlayerConnected
			}

			msgChannel, err := s.Channel(m.ChannelID)
			if err != nil {
				return err
			}

			msgGuild, err := s.Guild(msgChannel.GuildID)
			if err != nil {
				return err
			}

			for _, vState := range msgGuild.VoiceStates {
				if vState.UserID == m.Author.ID {
					var err error
					player.VoiceConnection, err = s.ChannelVoiceJoin(gID, vState.ChannelID, false, false)
					if err != nil {
						return err
					}
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
		RunFunc: func(raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
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

			song, err := player.Queue.Get(id)
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

	RegisterCommands(&queueSong, &queueList, &skip, &stop, &playlist, &move, &remove, &info, &join)

	go func() {
		for {
			select {
			case item := <-player.QueueChannel:
				player.Queue.Add(item)

				if !player.IsPlaying {
					song, err := player.Queue.GetFirst()
					if err == nil {
						player.SongChannel <- song
					}
				}

			case <-player.NextChannel:
				player.Queue.Remove(0)
				song, err := player.Queue.GetFirst()
				if err == nil {
					player.SongChannel <- song
				}
			}
		}
	}()

	go func() {
		for {
			song := <-player.SongChannel

			player.DgoSession.UpdateStatus(0, song.Info.Title)
			player.DgoSession.ChannelTopicEdit(Config.TextChannel, "Playing: "+song.Info.Title)

			player.PlayStream(song.Stream)

			player.DgoSession.UpdateStatus(0, "")
			player.DgoSession.ChannelTopicEdit(Config.TextChannel, "")

			player.NextChannel <- true
		}
	}()
}

func (player *Player) PlayStream(stream Playable) {
	options := dca.EncodeOptions{
		Volume: 256,
		Channels: channels,
		FrameRate: frameRate,
		FrameDuration: 20,
		Bitrate: 128,
		PacketLoss: 5,
		RawOutput: true,
		Application: dca.AudioApplicationAudio,
		CompressionLevel: 8,
		BufferedFrames: 100,
		VBR: false,
	}
	encoder, err := dca.EncodeMem(stream.Play(), &options)
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

	player.IsPlaying = true
	defer func() { player.IsPlaying = false }()

	done := make(chan error)
	dca.NewStream(encoder, player.VoiceConnection, done)

	select {
	case <-player.StopChannel:
		stream.Stop()
		encoder.Stop()
		return
	//TODO pause
	}
}

func (player *Player) Stop() error {
	player.Queue.Purge()

	if player.IsPlaying {
		player.StopChannel <- true
	}

	if player.VoiceConnection != nil {
		// wait for the player to stop sending data
		for player.IsPlaying {
			runtime.Gosched()
		}

		err := player.VoiceConnection.Disconnect()
		if err != nil {
			return err
		}
	}

	player.VoiceConnection = nil

	return nil
}
