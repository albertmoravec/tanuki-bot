package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/bwmarrin/dgvoice"
	"github.com/bwmarrin/discordgo"
	"github.com/rylio/ytdl"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

type Player struct {
	IsPlaying       bool
	Queue           Queue
	QueueChannel    chan Playable
	SongChannel     chan QueueItem
	NextChannel     chan bool
	StopChannel     chan bool
	VoiceConnection *discordgo.VoiceConnection
	FFmpeg          *exec.Cmd
	SendChannel     chan []int16
	DgoSession      *discordgo.Session
}

const (
	channels  int = 2     // 1 for mono, 2 for stereo
	frameRate int = 48000 // audio sampling rate
	frameSize int = 960   // uint16 size of each audio frame
)

func InitPlayer(s *discordgo.Session, gID string, vID string) {
	voiceConn, err := s.ChannelVoiceJoin(gID, vID, false, false)
	if err != nil {
		fmt.Println(err)
		return
	}

	player := Player{
		IsPlaying:       false,
		Queue:           Queue{},
		SongChannel:     make(chan QueueItem),
		QueueChannel:    make(chan Playable),
		NextChannel:     make(chan bool),
		StopChannel:     make(chan bool),
		SendChannel:     make(chan []int16, 2),
		VoiceConnection: voiceConn,
		DgoSession:      s,
	}

	for player.VoiceConnection.Ready == false {
		runtime.Gosched()
	}

	//d.VoiceConnections[guild].LogLevel = discordgo.LogDebug

	queueSong := CommandConstructor{
		Names:             []string{"queue", "q", "p"},
		Permission:        "queue",
		DefaultPermission: true,
		NoArguments:       false,
		MinArguments:      1,
		MaxArguments:      -1,
		RunFunc: func(raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			youtubeRegexp := regexp.MustCompile(`youtu(?:be\.com/(?:v/|e(?:mbed)?/|watch\?v=)|\.be/)([\w-]{11}\b)`)

			for _, link := range raw {
				id := youtubeRegexp.FindStringSubmatch(link)

				if len(id) > 0 {
					log.Println("Match found")
					video, err := ytdl.GetVideoInfo(id[1])
					if err != nil {
						log.Print(err)
					}
					log.Println("Sending to channel")
					player.QueueChannel <- YoutubeItem{video, nil}
					log.Println("Sent successfuly")
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
		MaxArguments:      1,
		RunFunc: func(raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			queueRegexp := regexp.MustCompile(`youtube\.com/(?:playlist\?list=|watch\?v=[\w-]{11}(?:&index=\d+)*&list=)([\w-]{34}\b)`)

			if !queueRegexp.MatchString(raw[0]) {
				s.ChannelMessageSend(m.ChannelID, "No playlist matched")
			}

			s.ChannelMessageSend(m.ChannelID, "Playlist matched")

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
			log.Println("Requesting skip")
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
			player.Stop()

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
		RunFunc: func(_ []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			var formatedList string

			queue, err := player.Queue.GetAll()
			if err != nil {
				return err
			}

			for pos, item := range queue {
				formatedList = strings.Join([]string{formatedList, strconv.Itoa(pos + 1), ". ", item.Info.Title, "\n"}, "")
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
	/*join := CommandConstructor{
		Names:             []string{"join", "j"},
		Permission:        "join",
		DefaultPermission: true,
		NoArguments:       true,
		MinArguments:      0,
		MaxArguments:      -1,
		RunFunc: func(raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
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
					//InitPlayer(s, vState.GuildID, vState.ChannelID)
				}
			}

			return nil
		},
	}*/

	RegisterCommands(&queueSong, &queueList, &skip, &stop, &playlist, &move, &remove/*, &join*/)

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
			case <-player.StopChannel:
				return
			}
		}
	}()

	go func() {
		for {
			select {
			case song := <-player.SongChannel:
				player.IsPlaying = true

				player.DgoSession.UpdateStatus(0, song.Info.Title)
				player.DgoSession.ChannelTopicEdit(Config.TextChannel, "Playing: "+song.Info.Title)

				player.PlayStream(song.Stream)

				player.DgoSession.UpdateStatus(0, "")
				player.DgoSession.ChannelTopicEdit(Config.TextChannel, "")

				player.IsPlaying = false

				player.NextChannel <- true
			case <-player.StopChannel:
				player.DgoSession.UpdateStatus(0, "")
				player.DgoSession.ChannelTopicEdit(Config.TextChannel, "")
				player.IsPlaying = false
				return
			}
		}
	}()
}

func (player *Player) PlayStream(stream Playable) {
	player.FFmpeg = exec.Command("ffmpeg", "-hide_banner", "-loglevel", "fatal", "-i", "pipe:0", "-af", "dynaudnorm", "-f", "s16le", "-ar", strconv.Itoa(frameRate), "-ac", strconv.Itoa(channels), "pipe:1")
	player.FFmpeg.Stdin = stream.Play()
	player.FFmpeg.Stderr = os.Stderr
	ffmpegOut, err := player.FFmpeg.StdoutPipe()
	if err != nil {
		fmt.Println("FFmpeg StdoutPipe Error:", err)
		return
	}
	ffmpegBuffer := bufio.NewReaderSize(ffmpegOut, 65536)

	err = player.FFmpeg.Start()
	if err != nil {
		fmt.Println("FFmpeg RunStart Error:", err)
		return
	}
	defer func() {
		go player.FFmpeg.Wait()
	}()

	player.VoiceConnection.Speaking(true)
	defer player.VoiceConnection.Speaking(false)

	go dgvoice.SendPCM(player.VoiceConnection, player.SendChannel)

	audiobuf := make([]int16, frameSize*channels)

	player.IsPlaying = true
	defer func() { player.IsPlaying = false }()
	for {
		select {
		case <-player.StopChannel:
			//stream.Stop() //TODO make the stream source cancelable (context maybe?)
			player.FFmpeg.Process.Kill()
			return
		default:
			err = binary.Read(ffmpegBuffer, binary.LittleEndian, &audiobuf)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				fmt.Println("Stream end")
				return
			}
			if err != nil {
				fmt.Println("Error reading from FFmpeg stdout:", err)
				return
			}

			player.SendChannel <- audiobuf
		}
	}
}

func (player *Player) Stop() {
	player.Queue.Purge()

	if player.IsPlaying {
		player.StopChannel <- true
	}

	err := player.VoiceConnection.Disconnect()
	if err != nil {
		log.Println(err)
	}
}
