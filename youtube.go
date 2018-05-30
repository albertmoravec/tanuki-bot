package main

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/rylio/ytdl"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/youtube/v3"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
)

type YoutubeItem struct {
	Video   *ytdl.VideoInfo
	ytdlCmd *exec.Cmd
}

func (yt *YoutubeItem) Play() io.Reader {
	//TODO YTDL returning early
	/*reader, writer := io.Pipe()
	format := yt.Video.Formats.Extremes(ytdl.FormatAudioBitrateKey, true)[0]
	go func() {
		url, err := yt.Video.GetDownloadURL(format)
		if err != nil {
			log.Println("error: " + err.Error())
		}

		resp, err := http.Get(url.String())
		if err != nil {
			log.Println("error: " + err.Error())
		}
		defer resp.Body.Close()

		_, err = io.Copy(writer, resp.Body)
		if err != nil {
			log.Println("error: " + err.Error())
		}
		log.Println("Download returned")
	}()

	return io.Reader(bufio.NewReaderSize(reader, 65536))
	*/
	
	/* using youtube-dl for now */
	yt.ytdlCmd = exec.Command("youtube-dl", "-o", "-", yt.Video.ID)
	yt.ytdlCmd.Stderr = os.Stderr
	youtubedlOut, err := yt.ytdlCmd.StdoutPipe()
	if err != nil {
		fmt.Println("YTDL StdoutPipe Error:", err)
		return nil
	}

	err = yt.ytdlCmd.Start()
	if err != nil {
		fmt.Println("YTDL StdoutPipe Error:", err)
		return nil
	}
	return bufio.NewReaderSize(youtubedlOut, 65536)
}

func (yt *YoutubeItem) Stop() {
	if yt.ytdlCmd.Process != nil {
		yt.ytdlCmd.Process.Kill()
	}
}

func (yt *YoutubeItem) GetInfo() ItemInfo {
	return ItemInfo{
		Title:    yt.Video.Title,
		Link:     "http://youtu.be/" + yt.Video.ID,
		Duration: yt.Video.Duration.String(),
	}
}

func CreateYoutubeItem(url string) (*YoutubeItem, error) {
	video, err := ytdl.GetVideoInfo(url)
	if err != nil {
		return nil, err
	}

	return &YoutubeItem{video, nil}, nil
}

func CreateQueueItem(url, requested string) (*QueueItem, error) {
	video, err := CreateYoutubeItem(url)
	if err != nil {
		return nil, err
	}

	return &QueueItem{
		Stream:      video,
		Info:        video.GetInfo(),
		RequestedBy: requested,
	}, nil
}

func RetrievePlaylist(service *youtube.Service, url string, requested string, items chan *QueueItem) error {
	playlistItems, err := service.PlaylistItems.List("snippet").PlaylistId(url).MaxResults(50).Do()
	if err != nil {
		return err
	}

	go func() {
		defer close(items)

		for _, video := range playlistItems.Items {
			item, err := CreateQueueItem(video.Snippet.ResourceId.VideoId, requested)
			if err != nil {
				log.Println(err)
				continue
			}

			items <- item
		}
	}()

	return nil
}

func Find(service *youtube.Service, query string, requested string) (*QueueItem, error) {
	videos, err := service.Search.List("snippet").Q(query).Type("video").MaxResults(1).Do()
	if err != nil {
		return nil, err
	}

	if len(videos.Items) == 0 {
		return nil, errors.New("No video found")
	}

	item, err := CreateQueueItem(videos.Items[0].Id.VideoId, requested)
	if err != nil {
		return nil, err
	}

	return item, nil
}

func LoadYoutubeAPIConfig(filePath string) (*jwt.Config, error) {
	token, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return google.JWTConfigFromJSON(token, youtube.YoutubeScope)
}
